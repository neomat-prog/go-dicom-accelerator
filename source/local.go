package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/suyashkumar/dicom"
	dicomtag "github.com/suyashkumar/dicom/pkg/tag"
)

type LocalSource struct {
	DICOMFilePath string
}

func NewLocal(dicomFilePath string) *LocalSource {
	return &LocalSource{DICOMFilePath: dicomFilePath}
}

// StudyMetadata reads the DICOM identifiers from the configured file.
func (s *LocalSource) StudyMetadata(ctx context.Context, studyUID string) (Metadata, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}

	metadata, err := readLocalMetadata(s.DICOMFilePath)
	if err != nil {
		return Metadata{}, Wrap(ErrorKindUpstream, err)
	}

	if studyUID != "" && studyUID != metadata.StudyInstanceUID {
		return Metadata{}, Wrap(ErrorKindNotFound, fmt.Errorf("study %q not found", studyUID))
	}
	return metadata, nil
}

// Instance opens the configured DICOM file when ref matches its identifiers.
func (s *LocalSource) Instance(ctx context.Context, ref InstanceRef) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}

	metadata, err := readLocalMetadata(s.DICOMFilePath)
	if err != nil {
		return Response{}, Wrap(ErrorKindUpstream, err)
	}

	if ref.StudyInstanceUID != "" && ref.StudyInstanceUID != metadata.StudyInstanceUID {
		return Response{}, Wrap(ErrorKindNotFound, fmt.Errorf("study %q not found", ref.StudyInstanceUID))
	}
	if ref.SeriesInstanceUID != "" && ref.SeriesInstanceUID != metadata.SeriesInstanceUID {
		return Response{}, Wrap(ErrorKindNotFound, fmt.Errorf("series %q not found", ref.SeriesInstanceUID))
	}
	if ref.SOPInstanceUID != "" && ref.SOPInstanceUID != metadata.SOPInstanceUID {
		return Response{}, Wrap(ErrorKindNotFound, fmt.Errorf("instance %q not found", ref.SOPInstanceUID))
	}

	file, err := os.Open(s.DICOMFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Response{}, Wrap(ErrorKindNotFound, err)
		}
		return Response{}, Wrap(ErrorKindUpstream, err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return Response{}, Wrap(ErrorKindUpstream, err)
	}

	return Response{
		Body:          file,
		ContentType:   "application/dicom",
		ContentLength: info.Size(),
		Filename:      filepath.Base(s.DICOMFilePath),
	}, nil
}

func readLocalMetadata(dicomFilePath string) (Metadata, error) {
	ds, err := dicom.ParseFile(dicomFilePath, nil, dicom.SkipPixelData())
	if err != nil {
		return Metadata{}, fmt.Errorf("parse dicom: %w", err)
	}

	studyUID, err := readStringTag(ds, dicomtag.StudyInstanceUID)
	if err != nil {
		return Metadata{}, err
	}

	seriesUID, err := readStringTag(ds, dicomtag.SeriesInstanceUID)
	if err != nil {
		return Metadata{}, err
	}

	instanceUID, err := readStringTag(ds, dicomtag.SOPInstanceUID)
	if err != nil {
		return Metadata{}, err
	}

	return Metadata{
		StudyInstanceUID:  studyUID,
		SeriesInstanceUID: seriesUID,
		SOPInstanceUID:    instanceUID,
	}, nil
}

func readStringTag(ds dicom.Dataset, t dicomtag.Tag) (string, error) {
	elem, err := ds.FindElementByTag(t)
	if err != nil {
		return "", fmt.Errorf("missing tag %v: %w", t, err)
	}

	values := dicom.MustGetStrings(elem.Value)
	if len(values) == 0 {
		return "", fmt.Errorf("tag %v has no value", t)
	}

	return values[0], nil
}
