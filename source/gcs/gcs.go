package gcs

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/neomat-prog/go-dicom-gateway/source"
	"google.golang.org/api/iterator"
)

type Source struct {
	bucket *storage.BucketHandle
	prefix string
}

func New(ctx context.Context, bucketName, prefix string) (*Source, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, source.Wrap(source.ErrorKindUpstream, fmt.Errorf("create gcs client: %w", err))
	}
	return &Source{
		bucket: client.Bucket(bucketName),
		prefix: prefix,
	}, nil
}

func (s *Source) Probe(ctx context.Context) error {
	it := s.bucket.Objects(ctx, &storage.Query{Prefix: s.prefix})
	if _, err := it.Next(); err != nil && err != iterator.Done {
		return source.Wrap(source.ErrorKindUpstream, fmt.Errorf("gcs bucket not accessible: %w", err))
	}
	return nil
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

// objectName builds the GCS object path from an InstanceRef.
// Convention: {prefix}{studyUID}/{seriesUID}/{sopUID}.dcm
func (s *Source) objectName(ref source.InstanceRef) string {
	return s.prefix + ref.StudyInstanceUID + "/" + ref.SeriesInstanceUID + "/" + ref.SOPInstanceUID + ".dcm"
}

// parseRefFromObjectName extracts UIDs from a path like:
// "prefix/studyUID/seriesUID/sopUID.dcm"
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

func (s *Source) SeriesInstances(ctx context.Context, studyUID, seriesUID string) ([]source.InstanceInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	objects, err := s.listObjects(ctx)
	if err != nil {
		return nil, err
	}

	var instances []source.InstanceInfo
	for _, attrs := range objects {
		ref, ok := parseRefFromObjectName(attrs.Name)
		if !ok {
			continue
		}
		if studyUID != "" && ref.StudyInstanceUID != studyUID {
			continue
		}
		if seriesUID != "" && ref.SeriesInstanceUID != seriesUID {
			continue
		}
		instances = append(instances, source.InstanceInfo{Ref: ref})
	}

	if len(instances) == 0 {
		return nil, source.Wrap(source.ErrorKindNotFound, fmt.Errorf("series not found"))
	}

	source.SortInstanceInfos(instances)
	return instances, nil
}

func (s *Source) StudySeries(ctx context.Context, studyUID string) ([]source.SeriesInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	objects, err := s.listObjects(ctx)
	if err != nil {
		return nil, err
	}

	seriesByUID := make(map[string]*source.SeriesInfo)

	for _, attrs := range objects {
		ref, ok := parseRefFromObjectName(attrs.Name)
		if !ok {
			continue
		}
		if studyUID != "" && ref.StudyInstanceUID != studyUID {
			continue
		}

		key := ref.StudyInstanceUID + "\x00" + ref.SeriesInstanceUID

		series, ok := seriesByUID[key]
		if !ok {
			series = &source.SeriesInfo{
				StudyInstanceUID:  ref.StudyInstanceUID,
				SeriesInstanceUID: ref.SeriesInstanceUID,
			}
			seriesByUID[key] = series
		}
		series.Instances = append(series.Instances, source.InstanceInfo{Ref: ref})
	}

	if len(seriesByUID) == 0 {
		return nil, source.Wrap(source.ErrorKindNotFound, fmt.Errorf("study not found"))
	}

	seriesList := make([]source.SeriesInfo, 0, len(seriesByUID))
	for _, series := range seriesByUID {
		source.SortInstanceInfos(series.Instances)
		seriesList = append(seriesList, *series)
	}

	source.SortSeriesList(seriesList)
	return seriesList, nil
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
