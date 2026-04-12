package httpapi

import "net/http"

// NewMux registers the HTTP routes exposed by the application.
func NewMux(dicomFilePath string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/dicom", dicomHandler(dicomFilePath))
	mux.HandleFunc("/dicom/metadata", dicomMetadataHandler(dicomFilePath))
	return mux
}
