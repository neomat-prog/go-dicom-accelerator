package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	mux := NewMux("")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected Content-Type starting with text/plain, got %q", ct)
	}

	if !strings.Contains(w.Body.String(), "\"status\":\"ok\"") {
        t.Fatalf("expected body to contain status ok, got %q", w.Body.String())
    }
}