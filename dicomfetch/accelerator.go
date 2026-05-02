package dicomfetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/neomat-prog/go-dicom-gateway/source"
)

/*
	WindowBehind = how many previous instances to warm
	WindowAhead = how many next instances to prefetch
*/

type Options struct {
	MaxConcurrency int
	BatchSize      int
	WindowAhead    int
	WindowBehind   int
	RequestTimeout time.Duration
}

type Fetcher struct {
	Source  source.Source
	Options Options
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
	return o
}

func New(src source.Source, options Options) *Fetcher {
	return &Fetcher{
		Source:  src,
		Options: options.Normalize(),
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

func (f *Fetcher) FetchInstance(ctx context.Context, ref source.InstanceRef) (source.Response, error) {
	if f == nil {
		return source.Response{}, errors.New("fetcher cannot be nil")
	}

	if f.Source == nil {
		return source.Response{}, errors.New("fetcher source cannot be nil")
	}

	if f.Options.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, f.Options.RequestTimeout)
		defer cancel()
	}

	return f.Source.Instance(ctx, ref)
}

func (f *Fetcher) FetchWindow(ctx context.Context, refs []source.InstanceRef, center int) ([]source.Response, error) {

	if f == nil {
		return nil, errors.New("dicomfetch: nil fetcher")
	}

	options := f.Options.Normalize()

	window, err := SelectWindow(refs, center, options.WindowBehind, options.WindowAhead)

	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	responses := make([]source.Response, len(window))
	sem := make(chan struct{}, options.MaxConcurrency)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for i, ref := range window {
		select {
		case <-ctx.Done():
			break
		case sem <- struct{}{}:
		}

		wg.Add(1)

		go func(i int, ref source.InstanceRef) {
			defer wg.Done()
			defer func() { <-sem }()

			resp, err := f.FetchInstance(ctx, ref)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("fetch instance %q: %w", ref.SOPInstanceUID, err))
				mu.Unlock()

				cancel()
				return
			}

			responses[i] = resp
		}(i, ref)
	}

	wg.Wait()

	if len(errs) > 0 {
		closeResponses(responses)
		return nil, errors.Join(errs...)
	}

	if err := ctx.Err(); err != nil {
		closeResponses(responses)
		return nil, err
	}

	return responses, nil

}

func closeResponses(responses []source.Response) {
	for _, resp := range responses {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}
}

func ReadAllAndClose(resp source.Response) ([]byte, error) {
	if resp.Body == nil {
		return nil, errors.New("dicomfetch: response body is nil")
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
