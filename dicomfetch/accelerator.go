package dicomfetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/neomat-prog/go-dicom-gateway/source"
)

type Options struct {
	MaxConcurrency int
	BatchSize      int
	WindowAhead    int
	WindowBehind   int
	RequestTimeout time.Duration
	MaxCacheBytes  int64
}

type Fetcher struct {
	Source  source.Source
	Options Options

	mut        sync.Mutex
	cache      map[string]FetchedInstance
	cacheBytes int64
}

type FetchedInstance struct {
	Ref         source.InstanceRef
	ContentType string
	Filename    string
	Data        []byte
}

func DefaultOptions() Options {
	return Options{
		MaxConcurrency: 8,
		BatchSize:      32,
		WindowAhead:    16,
		WindowBehind:   4,
		RequestTimeout: 15 * time.Second,
	}
}

func (o Options) Normalize() Options {
	if o.MaxConcurrency <= 0 {
		o.MaxConcurrency = 1
	}
	if o.BatchSize < 0 {
		o.BatchSize = 0
	}
	if o.WindowAhead < 0 {
		o.WindowAhead = 0
	}
	if o.WindowBehind < 0 {
		o.WindowBehind = 0
	}
	if o.RequestTimeout < 0 {
		o.RequestTimeout = 0
	}
	if o.MaxCacheBytes < 0 {
		o.MaxCacheBytes = 0
	}
	return o
}

func New(src source.Source, options Options) *Fetcher {
	return &Fetcher{
		Source:  src,
		Options: options.Normalize(),
		cache:   make(map[string]FetchedInstance),
	}
}

// CacheSize returns the number of cached instances.
func (f *Fetcher) CacheSize() int {
	if f == nil {
		return 0
	}
	f.mut.Lock()
	defer f.mut.Unlock()
	return len(f.cache)
}

// CacheBytes returns the number of DICOM bytes currently held in cache.
func (f *Fetcher) CacheBytes() int64 {
	if f == nil {
		return 0
	}
	f.mut.Lock()
	defer f.mut.Unlock()
	return f.cacheBytes
}

// Response converts the fetched bytes into a source.Response with a fresh
// reader.
func (i FetchedInstance) Response() source.Response {
	return source.Response{
		Body:          io.NopCloser(bytes.NewReader(i.Data)),
		ContentType:   i.ContentType,
		ContentLength: int64(len(i.Data)),
		Filename:      i.Filename,
	}
}

/*
Example usage:
	refs:   0 1 2 3 4 5 6 7 8 9
	center: 4
	behind: 2
	ahead:  3

	result: 2 3 4 5 6 7
*/

func SelectWindow(refs []source.InstanceRef, center int, behind int, ahead int) ([]source.InstanceRef, error) {
	if len(refs) == 0 {
		return nil, errors.New("refs cannot be empty")
	}

	if center < 0 || center >= len(refs) {
		return nil, fmt.Errorf("center index %d out of range [0,%d)", center, len(refs))
	}

	if behind < 0 {
		return nil, errors.New("behind cannot be negative")
	}

	if ahead < 0 {
		return nil, errors.New("ahead cannot be negative")
	}

	start := center - behind

	if start < 0 {
		start = 0
	}

	end := center + ahead + 1

	if end > len(refs) {
		end = len(refs)
	}

	window := make([]source.InstanceRef, end-start)
	copy(window, refs[start:end])

	return window, nil
}

// FetchInstance returns one DICOM instance, using the cache when possible.
func (f *Fetcher) FetchInstance(ctx context.Context, ref source.InstanceRef) (FetchedInstance, error) {
	if f == nil {
		return FetchedInstance{}, errors.New("fetcher cannot be nil")
	}

	if f.Source == nil {
		return FetchedInstance{}, errors.New("fetcher source cannot be nil")
	}

	if got, ok := f.getCached(ref); ok {
		log.Printf("dicomfetch: cache hit sop=%s bytes=%d", ref.SOPInstanceUID, len(got.Data))
		return got, nil
	}

	if f.Options.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, f.Options.RequestTimeout)
		defer cancel()
	}

	log.Printf("dicomfetch: cache miss sop=%s", ref.SOPInstanceUID)

	resp, err := f.Source.Instance(ctx, ref)
	if err != nil {
		return FetchedInstance{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return FetchedInstance{}, err
	}

	got := FetchedInstance{
		Ref:         ref,
		ContentType: resp.ContentType,
		Filename:    resp.Filename,
		Data:        data,
	}

	f.setCached(got)
	log.Printf("dicomfetch: fetched sop=%s bytes=%d", got.Ref.SOPInstanceUID, len(got.Data))
	return cloneFetchedInstance(got), nil
}

// FetchWindow fetches a sliding window of instances around center.
func (f *Fetcher) FetchWindow(ctx context.Context, refs []source.InstanceRef, center int) ([]FetchedInstance, error) {
	if f == nil {
		return nil, errors.New("dicomfetch: nil fetcher")
	}

	window, err := SelectWindow(refs, center, f.Options.WindowBehind, f.Options.WindowAhead)

	if err != nil {
		return nil, err
	}

	log.Printf(
		"dicomfetch: fetch window center=%d window=%d behind=%d ahead=%d concurrency=%d",
		center,
		len(window),
		f.Options.WindowBehind,
		f.Options.WindowAhead,
		f.Options.MaxConcurrency,
	)

	return f.fetchRefs(ctx, window, "window")
}

// FetchSeries fetches all refs in a series with bounded concurrency.
func (f *Fetcher) FetchSeries(ctx context.Context, refs []source.InstanceRef) ([]FetchedInstance, error) {
	if f == nil {
		return nil, errors.New("dicomfetch: nil fetcher")
	}

	if len(refs) == 0 {
		return []FetchedInstance{}, nil
	}

	log.Printf(
		"dicomfetch: fetch series series=%s instances=%d concurrency=%d",
		refs[0].SeriesInstanceUID,
		len(refs),
		f.Options.MaxConcurrency,
	)

	return f.fetchRefs(ctx, refs, "series")
}

// WarmSeries fetches refs into cache and returns loaded bytes and completed
// instance count.
func (f *Fetcher) WarmSeries(ctx context.Context, refs []source.InstanceRef) (int64, int, error) {
	if f == nil {
		return 0, 0, errors.New("dicomfetch: nil fetcher")
	}
	if len(refs) == 0 {
		return 0, 0, nil
	}

	sem := make(chan struct{}, f.Options.MaxConcurrency)

	var wg sync.WaitGroup
	var mut sync.Mutex
	var errs []error
	var bytesLoaded int64
	var completed int

outer:
	for _, ref := range refs {
		select {
		case <-ctx.Done():
			break outer
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(ref source.InstanceRef) {
			defer wg.Done()
			defer func() { <-sem }()
			fi, err := f.FetchInstance(ctx, ref)
			mut.Lock()
			defer mut.Unlock()
			if err != nil {
				errs = append(errs, fmt.Errorf("warm instance %q: %w", ref.SOPInstanceUID, err))
				return
			}
			bytesLoaded += int64(len(fi.Data))
			fi.Data = nil // cache already holds its clone
			completed++
		}(ref)
	}
	wg.Wait()
	return bytesLoaded, completed, errors.Join(errs...)
}

// ReadAllAndClose reads resp.Body and always closes it.
func ReadAllAndClose(resp source.Response) ([]byte, error) {
	if resp.Body == nil {
		return nil, errors.New("dicomfetch: response body is nil")
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (f *Fetcher) fetchRefs(ctx context.Context, refs []source.InstanceRef, label string) ([]FetchedInstance, error) {
	if f == nil {
		return nil, errors.New("dicomfetch: nil fetcher")
	}

	if len(refs) == 0 {
		return []FetchedInstance{}, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	instances := make([]FetchedInstance, len(refs))
	sem := make(chan struct{}, f.Options.MaxConcurrency)

	var wg sync.WaitGroup
	var mut sync.Mutex
	var errs []error
outer:
	for i, ref := range refs {
		select {
		case <-ctx.Done():
			break outer
		case sem <- struct{}{}:
		}
		wg.Add(1)

		go func(i int, ref source.InstanceRef) {
			defer wg.Done()
			defer func() { <-sem }()

			log.Printf("dicomfetch: %s fetch start index=%d sop=%s", label, i, ref.SOPInstanceUID)

			instance, err := f.FetchInstance(ctx, ref)
			if err != nil {
				mut.Lock()
				errs = append(errs, fmt.Errorf("fetch instance %q: %w", ref.SOPInstanceUID, err))
				mut.Unlock()

				cancel()
				return
			}

			log.Printf("dicomfetch: %s fetch done index=%d sop=%s bytes=%d", label, i, instance.Ref.SOPInstanceUID, len(instance.Data))
			instances[i] = instance
		}(i, ref)
	}

	wg.Wait()

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return instances, nil
}

func (f *Fetcher) getCached(ref source.InstanceRef) (FetchedInstance, bool) {
	f.mut.Lock()
	defer f.mut.Unlock()

	got, ok := f.cache[cacheKey(ref)]
	if !ok {
		return FetchedInstance{}, false
	}
	return cloneFetchedInstance(got), true
}

func (f *Fetcher) setCached(instance FetchedInstance) {
	f.mut.Lock()
	defer f.mut.Unlock()

	if f.cache == nil {
		f.cache = make(map[string]FetchedInstance)
	}
	key := cacheKey(instance.Ref)
	if existing, ok := f.cache[key]; ok {
		f.cacheBytes -= int64(len(existing.Data))
	}
	cloned := cloneFetchedInstance(instance)
	newBytes := int64(len(cloned.Data))
	if f.Options.MaxCacheBytes > 0 {
		for f.cacheBytes+newBytes > f.Options.MaxCacheBytes && len(f.cache) > 0 {
			for k, v := range f.cache {
				f.cacheBytes -= int64(len(v.Data))
				delete(f.cache, k)
				break
			}
		}
	}
	f.cache[key] = cloned
	f.cacheBytes += newBytes
}

func cloneFetchedInstance(instance FetchedInstance) FetchedInstance {
	instance.Data = append([]byte(nil), instance.Data...)
	return instance
}

func cacheKey(ref source.InstanceRef) string {
	return ref.StudyInstanceUID + "\x00" + ref.SeriesInstanceUID + "\x00" + ref.SOPInstanceUID
}
