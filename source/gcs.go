package source

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

type GCSSource struct {
	bucket *storage.BucketHandle
	prefix string
}

// Credentials come from GOOGLE_APPLICATION_CREDENTIALS or `gcloud auth application-default login`.
func NewGCSSource(ctx context.Context, bucketName, prefix string) (*GCSSource, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, Wrap(ErrorKindUpstream, fmt.Errorf("create gcs client: %w", err))
	}
	return &GCSSource{
		bucket: client.Bucket(bucketName),
		prefix: prefix,
	}, nil
}

func (s *GCSSource) Probe(ctx context.Context) error {
	_, err := s.bucket.Attrs(ctx)
	if err != nil {
		return Wrap(ErrorKindUpstream, fmt.Errorf("gcs bucket not accessible: %w", err))
	}
	return nil
}

func (s *GCSSource) listObjects(ctx context.Context) ([]*storage.ObjectAttrs, error) {
	var objects []*storage.ObjectAttrs

	it := s.bucket.Objects(ctx, &storage.Query{Prefix: s.prefix})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, Wrap(ErrorKindUpstream, fmt.Errorf("list gcs objects: %w", err))
		}
		if strings.HasSuffix(attrs.Name, ".dcm") {
			objects = append(objects, attrs)
		}
	}

	if len(objects) == 0 {
		return nil, Wrap(ErrorKindNotFound, fmt.Errorf("no dicom objects found under prefix %q", s.prefix))
	}
	return objects, nil
}

// Convention: {prefix}{studyUID}/{seriesUID}/{sopUID}.dcm
func (s *GCSSource) objectName(ref InstanceRef) string {
	return s.prefix + ref.StudyInstanceUID + "/" + ref.SeriesInstanceUID + "/" + ref.SOPInstanceUID + ".dcm"
}

func parseRefFromObjectName(name string) (InstanceRef, bool) {
	trimmed := strings.TrimSuffix(name, ".dcm")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 3 {
		return InstanceRef{}, false
	}
	n := len(parts)
	return InstanceRef{
		StudyInstanceUID:  parts[n-3],
		SeriesInstanceUID: parts[n-2],
		SOPInstanceUID:    parts[n-1],
	}, true
}

func (s *GCSSource) SeriesInstances(ctx context.Context, studyUID, seriesUID string) ([]InstanceInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	objects, err := s.listObjects(ctx)
	if err != nil {
		return nil, err
	}

	var instances []InstanceInfo
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
		instances = append(instances, InstanceInfo{Ref: ref})
	}

	if len(instances) == 0 {
		return nil, Wrap(ErrorKindNotFound, fmt.Errorf("series not found"))
	}

	sortInstanceInfos(instances)
	return instances, nil
}

func (s *GCSSource) StudySeries(ctx context.Context, studyUID string) ([]SeriesInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	objects, err := s.listObjects(ctx)
	if err != nil {
		return nil, err
	}

	seriesByUID := make(map[string]*SeriesInfo)

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
			series = &SeriesInfo{
				StudyInstanceUID:  ref.StudyInstanceUID,
				SeriesInstanceUID: ref.SeriesInstanceUID,
			}
			seriesByUID[key] = series
		}
		series.Instances = append(series.Instances, InstanceInfo{Ref: ref})
	}

	if len(seriesByUID) == 0 {
		return nil, Wrap(ErrorKindNotFound, fmt.Errorf("study not found"))
	}

	seriesList := make([]SeriesInfo, 0, len(seriesByUID))
	for _, series := range seriesByUID {
		sortInstanceInfos(series.Instances)
		seriesList = append(seriesList, *series)
	}

	sortSeriesList(seriesList)
	return seriesList, nil
}

func (s *GCSSource) StudyMetadata(ctx context.Context, studyUID string) (Metadata, error) {
	instances, err := s.SeriesInstances(ctx, studyUID, "")
	if err != nil {
		return Metadata{}, err
	}
	ref := instances[0].Ref
	return Metadata{
		StudyInstanceUID:  ref.StudyInstanceUID,
		SeriesInstanceUID: ref.SeriesInstanceUID,
		SOPInstanceUID:    ref.SOPInstanceUID,
	}, nil
}

func (s *GCSSource) Instance(ctx context.Context, ref InstanceRef) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}

	name := s.objectName(ref)
	obj := s.bucket.Object(name)

	attrs, err := obj.Attrs(ctx)
	if err == storage.ErrObjectNotExist {
		return Response{}, Wrap(ErrorKindNotFound, fmt.Errorf("instance not found %s", name))
	}
	if err != nil {
		return Response{}, Wrap(ErrorKindUpstream, fmt.Errorf("gcs attr %s, %w", name, err))
	}

	reader, err := obj.NewReader(ctx)
	if err != nil {
		return Response{}, Wrap(ErrorKindUpstream, fmt.Errorf("gcs open %s, %w", name, err))
	}

	return Response{
		Body:          reader,
		ContentType:   "application/dicom",
		ContentLength: attrs.Size,
		Filename:      ref.SOPInstanceUID + ".dcm",
	}, nil
}
