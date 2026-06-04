package httpapi_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/suyashkumar/dicom"
	dicomtag "github.com/suyashkumar/dicom/pkg/tag"
	"github.com/suyashkumar/dicom/pkg/uid"
)

const (
	testStudyUID  = "1.2.826.0.1.3680043.10.543.2"
	testSeriesUID = "1.2.826.0.1.3680043.10.543.3"
)

// writeDicom writes one instance into dir with the given SOP UID and number.
func writeDicom(t *testing.T, dir, sopUID string, instanceNumber int) {
	t.Helper()

	path := filepath.Join(dir, sopUID+".dcm")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()

	ds := dicom.Dataset{Elements: []*dicom.Element{
		mustElement(t, dicomtag.MediaStorageSOPClassUID, []string{"1.2.840.10008.5.1.4.1.1.7"}),
		mustElement(t, dicomtag.MediaStorageSOPInstanceUID, []string{sopUID}),
		mustElement(t, dicomtag.TransferSyntaxUID, []string{uid.ImplicitVRLittleEndian}),
		mustElement(t, dicomtag.StudyInstanceUID, []string{testStudyUID}),
		mustElement(t, dicomtag.SeriesInstanceUID, []string{testSeriesUID}),
		mustElement(t, dicomtag.SOPInstanceUID, []string{sopUID}),
		mustElement(t, dicomtag.InstanceNumber, []string{itoa(instanceNumber)}),
	}}

	if err := dicom.Write(f, ds); err != nil {
		t.Fatalf("write dicom: %v\n", err)
	}
}

func mustElement(t *testing.T, tag dicomtag.Tag, value any) *dicom.Element {
	t.Helper()
	elem, err := dicom.NewElement(tag, value)
	if err != nil {
		t.Fatalf("new element %v: %v", tag, err)
	}
	return elem
}

func itoa(n int) string { return strconv.Itoa(n) }
