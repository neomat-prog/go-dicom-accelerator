package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/neomat-prog/go-dicom-gateway/dicomfetch"
	"github.com/neomat-prog/go-dicom-gateway/source"
)

func healthHandler(sourceType string, prober source.Prober, fetcher *dicomfetch.Fetcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type sourceStatus struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Error  string `json:"error,omitempty"`
		}

		type response struct {
			Status string       `json:"status"`
			Source sourceStatus `json:"source"`
			Cache  struct {
				Size int `json:"size"`
			} `json:"cache"`
		}

		src := sourceStatus{Type: sourceType, Status: "ok"}
		if err := prober.Probe(r.Context()); err != nil {
			src.Status = "error"
			src.Error = err.Error()
		}

		status, code := "ok", http.StatusOK
		if src.Status != "ok" {
			status, code = "degraded", http.StatusServiceUnavailable
		}

		resp := response{Status: status, Source: src}
		resp.Cache.Size = fetcher.CacheSize()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
