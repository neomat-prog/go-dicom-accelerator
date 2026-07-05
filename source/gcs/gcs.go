package gcs

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/neomat-prog/go-dicom-gateway/source"
	"golang.org/x/sync/singleflight"
	"google.golang.org/api/iterator"
)

// DefaultIndexTTL bounds how long the object index is reused before relist
const DefaultIndexTTL = 30 * time.Second

type Source struct {
	bucket *storage.BucketHandle
	prefix string

	mu       sync.RWMutex
	index    *gcsIndex
	indexTTL time.Duration
	group    singleflight.Group
	now      func() time.Time
}

type gcsIndex struct {
	builtAt time.Time
	all     []source.InstanceInfo
	series  []source.SeriesInfo
}

func New(ctx context.Context, bucketName, prefix string) (*Source, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, source.Wrap(source.ErrorKindUpstream, fmt.Errorf("create gcs client: %w", err))
	}
	return &Source{
		bucket:   client.Bucket(bucketName),
		prefix:   prefix,
		indexTTL: DefaultIndexTTL,
	}, nil
}

func (s *Source) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Source) loadIndex(ctx context.Context) (*gcsIndex, error) {
	s.mu.RLock()
	idx := s.index
	s.mu.RUnlock()
	if idx != nil && s.clock().Sub(idx.builtAt) < s.indexTTL {
		return idx, nil
	}

	v, err, _ := s.group.Do("index", func() (any, error) {
		built, err := s.buildIndex(ctx)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.index = built
		s.mu.Unlock()
		return built, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*gcsIndex), nil
}

func (s *Source) buildIndex(ctx context.Context) (*gcsIndex, error) {
	objects, err := s.listObjects(ctx)
	if err != nil {
		return nil, err
	}

	idx := &gcsIndex{builtAt: s.clock()}
	seriesByUID := make(map[string]*source.SeriesInfo)

	for _, attrs := range objects {
		ref, ok := parseRefFromObjectName(attrs.Name)
		if !ok {
			continue
		}
		info := source.InstanceInfo{Ref: ref}
		idx.all = append(idx.all, info)

		key := ref.StudyInstanceUID + "\x00" + ref.SeriesInstanceUID
		series, ok := seriesByUID[key]
		if !ok {
			series = &source.SeriesInfo{
				StudyInstanceUID:  ref.StudyInstanceUID,
				SeriesInstanceUID: ref.SeriesInstanceUID,
			}
			seriesByUID[key] = series
		}
		series.Instances = append(series.Instances, info)
	}

	source.SortInstanceInfos(idx.all)
	for _, series := range seriesByUID {
		source.SortInstanceInfos(series.Instances)
		idx.series = append(idx.series, *series)
	}
	source.SortSeriesList(idx.series)
	return idx, nil
}

// Probe verifies that objects under the configured prefix can be listed.
func (s *Source) Probe(ctx context.Context) error {
	it := s.bucket.Objects(ctx, &storage.Query{Prefix: s.prefix})
	if _, err := it.Next(); err != nil && err != iterator.Done {
		return source.Wrap(source.ErrorKindUpstream, fmt.Errorf("gcs bucket not accessible: %w", err))
	}
	return nil
}

// SeriesInstances lists instances matching studyUID and seriesUID.
func (s *Source) SeriesInstances(ctx context.Context, studyUID, seriesUID string) ([]source.InstanceInfo, error) {
	idx, err := s.loadIndex(ctx)
	if err != nil {
		return nil, err
	}

	var instances []source.InstanceInfo
	for _, info := range idx.all {
		if studyUID != "" && info.Ref.StudyInstanceUID != studyUID {
			continue
		}
		if seriesUID != "" && info.Ref.SeriesInstanceUID != seriesUID {
			continue
		}
		instances = append(instances, info)
	}
	if len(instances) == 0 {
		return nil, source.Wrap(source.ErrorKindNotFound, fmt.Errorf("series not found"))
	}
	return instances, nil
}

// StudySeries groups GCS objects by study and series identifiers.
func (s *Source) StudySeries(ctx context.Context, studyUID string) ([]source.SeriesInfo, error) {
	idx, err := s.loadIndex(ctx)
	if err != nil {
		return nil, err
	}

	var out []source.SeriesInfo
	for _, series := range idx.series {
		if studyUID != "" && series.StudyInstanceUID != studyUID {
			continue
		}
		out = append(out, series)
	}
	if len(out) == 0 {
		return nil, source.Wrap(source.ErrorKindNotFound, fmt.Errorf("study not found"))
	}
	return out, nil
}

func (s *Source) StudyMetadata(ctx context.Context, studyUID string) (source.Metadata, error) {
	instances, err := s.SeriesInstances(ctx, studyUID, "")
	if err != nil {
		return source.Metadata{}, err
	}
	ref := instances[0].Ref
	return source.Metadata{
		StudyInstanceUID:  ref.StudyInstanceUID,
		SeriesInstanceUID: ref.SeriesInstanceUID,
		SOPInstanceUID:    ref.SOPInstanceUID,
	}, nil
}

func (s *Source) Instance(ctx context.Context, ref source.InstanceRef) (source.Response, error) {
	if err := ctx.Err(); err != nil {
		return source.Response{}, err
	}

	if !source.ValidUID(ref.StudyInstanceUID) || !source.ValidUID(ref.SeriesInstanceUID) || !source.ValidUID(ref.SOPInstanceUID) {
		return source.Response{}, source.Wrap(source.ErrorKindBadRequest, fmt.Errorf("invalid UID in instance ref"))
	}

	name := s.objectName(ref)
	obj := s.bucket.Object(name)

	attrs, err := obj.Attrs(ctx)
	if err == storage.ErrObjectNotExist {
		return source.Response{}, source.Wrap(source.ErrorKindNotFound, fmt.Errorf("instance not found: %s", name))
	}
	if err != nil {
		return source.Response{}, source.Wrap(source.ErrorKindUpstream, fmt.Errorf("gcs attrs %s: %w", name, err))
	}

	reader, err := obj.NewReader(ctx)
	if err != nil {
		return source.Response{}, source.Wrap(source.ErrorKindUpstream, fmt.Errorf("gcs open %s: %w", name, err))
	}

	return source.Response{
		Body:          reader,
		ContentType:   "application/dicom",
		ContentLength: attrs.Size,
		Filename:      ref.SOPInstanceUID + ".dcm",
	}, nil
}

func (s *Source) listObjects(ctx context.Context) ([]*storage.ObjectAttrs, error) {
	var objects []*storage.ObjectAttrs

	it := s.bucket.Objects(ctx, &storage.Query{Prefix: s.prefix})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, source.Wrap(source.ErrorKindUpstream, fmt.Errorf("list gcs objects: %w", err))
		}
		if strings.HasSuffix(attrs.Name, ".dcm") {
			objects = append(objects, attrs)
		}
	}

	if len(objects) == 0 {
		return nil, source.Wrap(source.ErrorKindNotFound, fmt.Errorf("no dicom objects found under prefix %q", s.prefix))
	}
	return objects, nil
}

func (s *Source) objectName(ref source.InstanceRef) string {
	return s.prefix + ref.StudyInstanceUID + "/" + ref.SeriesInstanceUID + "/" + ref.SOPInstanceUID + ".dcm"
}

func parseRefFromObjectName(name string) (source.InstanceRef, bool) {
	trimmed := strings.TrimSuffix(name, ".dcm")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 3 {
		return source.InstanceRef{}, false
	}
	n := len(parts)
	return source.InstanceRef{
		StudyInstanceUID:  parts[n-3],
		SeriesInstanceUID: parts[n-2],
		SOPInstanceUID:    parts[n-1],
	}, true
}
