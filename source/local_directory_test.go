package source

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/suyashkumar/dicom"
	dicomtag "github.com/suyashkumar/dicom/pkg/tag"
	"github.com/suyashkumar/dicom/pkg/uid"
)

func TestLocalDirectorySource_SeriesInstancesSortsByInstanceNumber(t *testing.T) {
	root := t.TempDir()
	writeDICOMFile(t, root, "third.dcm", "study-01", "series-01", "instance-03", "3")
	writeDICOMFile(t, root, "first.dcm", "study-01", "series-01", "instance-01", "1")
	writeDICOMFile(t, root, "second.dcm", "study-01", "series-01", "instance-02", "2")
	writeDICOMFile(t, root, "other-series.dcm", "study-01", "series-02", "instance-99", "1")
	writeNonDICOMFile(t, root, "notes.txt")

	src := NewLocalDirectory(root)

	got, err := src.SeriesInstances(context.Background(), "study-01", "series-01")
	if err != nil {
		t.Fatalf("SeriesInstances returned error: %v", err)
	}

	assertInstanceInfoUIDs(t, got, []string{
		"instance-01",
		"instance-02",
		"instance-03",
	})
}

func TestLocalDirectorySource_InstanceFetchesMatchingFile(t *testing.T) {
	root := t.TempDir()
	writeDICOMFile(t, root, "first.dcm", "study-01", "series-01", "instance-01", "1")
	writeDICOMFile(t, root, "second.dcm", "study-01", "series-01", "instance-02", "2")

	src := NewLocalDirectory(root)

	resp, err := src.Instance(context.Background(), InstanceRef{
		StudyInstanceUID:  "study-01",
		SeriesInstanceUID: "series-01",
		SOPInstanceUID:    "instance-02",
	})
	if err != nil {
		t.Fatalf("Instance returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.ContentType != "application/dicom" {
		t.Fatalf("expected application/dicom content type, got %q", resp.ContentType)
	}
	if resp.Filename != "second.dcm" {
		t.Fatalf("expected second.dcm filename, got %q", resp.Filename)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty DICOM body")
	}
}

func TestLocalDirectorySource_SeriesInstancesNotFound(t *testing.T) {
	root := t.TempDir()
	writeDICOMFile(t, root, "first.dcm", "study-01", "series-01", "instance-01", "1")

	src := NewLocalDirectory(root)

	_, err := src.SeriesInstances(context.Background(), "study-missing", "series-01")
	if !IsKind(err, ErrorKindNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func writeDICOMFile(t *testing.T, root string, name string, studyUID string, seriesUID string, instanceUID string, instanceNumber string) {
	t.Helper()

	path := filepath.Join(root, name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}

	ds := dicom.Dataset{Elements: []*dicom.Element{
		newDICOMElement(t, dicomtag.MediaStorageSOPClassUID, []string{"1.2.840.10008.5.1.4.1.1.7"}),
		newDICOMElement(t, dicomtag.MediaStorageSOPInstanceUID, []string{instanceUID + ".media"}),
		newDICOMElement(t, dicomtag.TransferSyntaxUID, []string{uid.ImplicitVRLittleEndian}),
		newDICOMElement(t, dicomtag.StudyInstanceUID, []string{studyUID}),
		newDICOMElement(t, dicomtag.SeriesInstanceUID, []string{seriesUID}),
		newDICOMElement(t, dicomtag.SOPInstanceUID, []string{instanceUID}),
		newDICOMElement(t, dicomtag.InstanceNumber, []string{instanceNumber}),
	}}

	if err := dicom.Write(file, ds); err != nil {
		t.Fatalf("write dicom %s: %v", path, err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

func writeNonDICOMFile(t *testing.T, root string, name string) {
	t.Helper()

	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte("not dicom"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newDICOMElement(t *testing.T, elementTag dicomtag.Tag, value any) *dicom.Element {
	t.Helper()

	elem, err := dicom.NewElement(elementTag, value)
	if err != nil {
		t.Fatalf("new dicom element %v: %v", elementTag, err)
	}

	return elem
}

func assertInstanceInfoUIDs(t *testing.T, instances []InstanceInfo, want []string) {
	t.Helper()

	if len(instances) != len(want) {
		t.Fatalf("expected %d instances, got %d", len(want), len(instances))
	}

	for i := range instances {
		if instances[i].Ref.SOPInstanceUID != want[i] {
			t.Fatalf("instance %d: expected %q, got %q", i, want[i], instances[i].Ref.SOPInstanceUID)
		}
	}
}
