package httpapi

import (
	"fmt"

	"github.com/suyashkumar/dicom"
	dicomtag "github.com/suyashkumar/dicom/pkg/tag"
)

func readDicomMetadata(dicomFilePath string) (DICOMMetadata, error) {
	ds, err := dicom.ParseFile(dicomFilePath, nil, dicom.SkipPixelData())
	if err != nil {
		return DICOMMetadata{}, fmt.Errorf("parse dicom: %w", err)
	}

	studyUID, err := readStringTag(ds, dicomtag.StudyInstanceUID)
	if err != nil {
		return DICOMMetadata{}, err
	}

	seriesUID, err := readStringTag(ds, dicomtag.SeriesInstanceUID)
	if err != nil {
		return DICOMMetadata{}, err
	}

	instanceUID, err := readStringTag(ds, dicomtag.SOPInstanceUID)
	if err != nil {
		return DICOMMetadata{}, err
	}

	return DICOMMetadata{
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
