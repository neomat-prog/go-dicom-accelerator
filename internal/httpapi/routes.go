package httpapi

import (
	"net/http"

	"github.com/neomat-prog/go-dicom-gateway/source"
)

// NewMux registers the HTTP routes exposed by the application.

// TODO(neomat-prog): instead of a file path use source.Source
// it should be source/adapter specific and not file path specific
func NewMux(src source.Source) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/dicom", dicomHandler(src))
	mux.HandleFunc("/dicom/metadata", dicomMetadataHandler(src))
	return mux
}
