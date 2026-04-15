package source

import (
	"context"
	"io"
	"errors"
)

// TODO(neomat-prog) maybe change this to InstanceUID?

type InstanceID struct{
	StudyInstanceUID string
	SeriesInstanceUID string
	SOPInstanceUID string
}

// convention to use ContentLength = -1 when length is unknown
type Response struct {
	Body io.ReadCloser
	ContentType string
	ContentLength int64
}

type Source interface {
	StudyMetadata(ctx context.Context, studyUID string) (Response, error)
 	Instance(ctx context.Context, id InstanceID) (Response, error)
}

type ErrorKind string

const (
	ErrorKindBadRequest    ErrorKind = "bad_request"
	ErrorKindNotFound      ErrorKind = "not_found"
	ErrorKindNotAcceptable ErrorKind = "not_acceptable"
	ErrorKindUpstream      ErrorKind = "upstream"
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

