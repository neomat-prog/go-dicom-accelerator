package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neomat-prog/go-dicom-gateway/dicomfetch"
	"github.com/neomat-prog/go-dicom-gateway/source"
)

type mockProber struct{ err error }

func (m mockProber) Probe(_ context.Context) error { return m.err }

func newTestMux() *http.ServeMux {
	src := source.NewLocal("")
	fetcher := dicomfetch.New(src, dicomfetch.DefaultOptions())
	return NewMux(src, "test", mockProber{}, fetcher)
}

func TestHealth(t *testing.T) {
	mux := newTestMux()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	if !strings.Contains(w.Body.String(), "\"status\":\"ok\"") {
		t.Fatalf("expected body to contain status ok, got %q", w.Body.String())
	}
}

func TestHealth_MethodNotAllowed(t *testing.T) {
	mux := newTestMux()

	req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}

	if !strings.Contains(w.Body.String(), "method not allowed") {
		t.Fatalf("expected method not allowed message, got %q", w.Body.String())
	}
}

func TestHealth_ResponseJSONContract(t *testing.T) {
	mux := newTestMux()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}

	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %v", body["status"])
	}

	src, ok := body["source"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected source object, got %T", body["source"])
	}
	if src["type"] != "test" {
		t.Fatalf("expected source.type=test, got %v", src["type"])
	}
	if src["status"] != "ok" {
		t.Fatalf("expected source.status=ok, got %v", src["status"])
	}

	cache, ok := body["cache"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected cache object, got %T", body["cache"])
	}
	if _, ok := cache["size"]; !ok {
		t.Fatalf("expected cache.size field")
	}
}

func TestHealth_Degraded(t *testing.T) {
	src := source.NewLocal("")
	fetcher := dicomfetch.New(src, dicomfetch.DefaultOptions())
	mux := NewMux(src, "test", mockProber{err: errors.New("root gone")}, fetcher)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}

	if body["status"] != "degraded" {
		t.Fatalf("expected status=degraded, got %v", body["status"])
	}

	src2, ok := body["source"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected source object")
	}
	if src2["status"] != "error" {
		t.Fatalf("expected source.status=error, got %v", src2["status"])
	}
	if src2["error"] != "root gone" {
		t.Fatalf("expected source.error=root gone, got %v", src2["error"])
	}
}
