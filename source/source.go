package source

import (
	"context"
	"errors"
	"io"
)

type InstanceRef struct {
	StudyInstanceUID  string
	SeriesInstanceUID string
	SOPInstanceUID    string
}

type InstanceInfo struct {
	Ref               InstanceRef
	InstanceNumber    int
	HasInstanceNumber bool
}

type SeriesLister interface {
	SeriesInstances(ctx context.Context, studyUID string, seriesUID string) ([]InstanceInfo, error)
}

type SeriesInfo struct {
	StudyInstanceUID  string
	SeriesInstanceUID string
	Instances         []InstanceInfo
}

type StudyLister interface {
	StudySeries(ctx context.Context, studyUID string) ([]SeriesInfo, error)
}

type Metadata struct {
	StudyInstanceUID  string `json:"studyInstanceUID"`
	SeriesInstanceUID string `json:"seriesInstanceUID"`
	SOPInstanceUID    string `json:"sopInstanceUID"`
}

// Use ContentLength = -1 when the length is unknown.
type Response struct {
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
	Filename      string
}

type Source interface {
	StudyMetadata(ctx context.Context, studyUID string) (Metadata, error)
	Instance(ctx context.Context, ref InstanceRef) (Response, error)
}

// Prober checks whether a backing DICOM source is reachable.
type Prober interface {
	Probe(ctx context.Context) error
}

type ErrorKind string

const (
	ErrorKindBadRequest ErrorKind = "bad_request"

	ErrorKindNotFound ErrorKind = "not_found"

	ErrorKindNotAcceptable ErrorKind = "not_acceptable"

	ErrorKindUpstream ErrorKind = "upstream"
)

type Error struct {
	Kind ErrorKind
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Kind)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func Wrap(kind ErrorKind, err error) error {
	return &Error{
		Kind: kind,
		Err:  err,
	}
}

func IsKind(err error, kind ErrorKind) bool {
	var target *Error
	if !errors.As(err, &target) {
		return false
	}
	return target.Kind == kind
}
