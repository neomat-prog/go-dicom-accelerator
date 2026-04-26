package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/suyashkumar/dicom"
	dicomtag "github.com/suyashkumar/dicom/pkg/tag"
	"github.com/suyashkumar/dicom/pkg/uid"
)

func TestDicomMetadataHandler_FileNotFound(t *testing.T) {
	handler := dicomMetadataHandler("testdata/does-not-exist.dcm")

	req := httptest.NewRequest(http.MethodGet, "/dicom/metadata", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected %d, got %d", http.StatusInternalServerError, w.Code)
	}

	body := w.Body.String()
	if body == "" {
		t.Fatalf("expected non-empty error body")
	}

	if !strings.Contains(body, "parse dicom") {
		t.Fatalf("expected parse error in body, got %q", body)
	}
}

func TestDicomMetadataHandler_FileExists(t *testing.T) {
	handler := dicomMetadataHandler(testDICOMFile(t))

	req := httptest.NewRequest(http.MethodGet, "/dicom/metadata", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d, body=%q", http.StatusOK, w.Code, w.Body.String())
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	var got DICOMMetadata
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode json response: %v", err)
	}

	if got.StudyInstanceUID == "" || got.SeriesInstanceUID == "" || got.SOPInstanceUID == "" {
		t.Fatalf("expected all UIDs to be non-empty, got %+v", got)
	}
}

func TestDicomHandler_FileOpens(t *testing.T) {
	handler := dicomHandler(testDICOMFile(t))

	req := httptest.NewRequest(http.MethodGet, "/dicom", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/dicom" {
		t.Fatalf("expected Content-Type application/dicom, got %q", ct)
	}

	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, `filename="test.dcm"`) {
		t.Fatalf("expected Content-Disposition filename test.dcm, got %q", cd)
	}

	if w.Body.Len() == 0 {
		t.Fatalf("expected non-empty body")
	}
}

func testDICOMFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.dcm")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}

	ds := dicom.Dataset{Elements: []*dicom.Element{
		newDICOMElement(t, dicomtag.MediaStorageSOPClassUID, []string{"1.2.840.10008.5.1.4.1.1.7"}),
		newDICOMElement(t, dicomtag.MediaStorageSOPInstanceUID, []string{"1.2.826.0.1.3680043.10.543.1"}),
		newDICOMElement(t, dicomtag.TransferSyntaxUID, []string{uid.ImplicitVRLittleEndian}),
		newDICOMElement(t, dicomtag.StudyInstanceUID, []string{"1.2.826.0.1.3680043.10.543.2"}),
		newDICOMElement(t, dicomtag.SeriesInstanceUID, []string{"1.2.826.0.1.3680043.10.543.3"}),
		newDICOMElement(t, dicomtag.SOPInstanceUID, []string{"1.2.826.0.1.3680043.10.543.4"}),
	}}

	if err := dicom.Write(file, ds); err != nil {
		t.Fatalf("write dicom %s: %v", path, err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}

	return path
}

func newDICOMElement(t *testing.T, elementTag dicomtag.Tag, value any) *dicom.Element {
	t.Helper()

	elem, err := dicom.NewElement(elementTag, value)
	if err != nil {
		t.Fatalf("new dicom element %v: %v", elementTag, err)
	}

	return elem
}

func TestDicomHandler_FileNotFound(t *testing.T) {
	handler := dicomHandler("testdata/does-not-exist.dcm")

	req := httptest.NewRequest(http.MethodGet, "/dicom", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected %d, got %d", http.StatusNotFound, w.Code)
	}
}
