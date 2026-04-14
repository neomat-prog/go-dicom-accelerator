package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	handler := dicomMetadataHandler("testdata/test.dcm")

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