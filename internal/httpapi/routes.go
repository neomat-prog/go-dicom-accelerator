package httpapi

import (
	"net/http"

	"github.com/neomat-prog/go-dicom-gateway/dicomfetch"
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

func NewAcceleratedMux(src source.Source, lister source.StudyLister, fetcher *dicomfetch.Fetcher, prefetcher *PrefetchManager) *http.ServeMux {
	mux := NewMux(src)

	mux.HandleFunc("GET /studies", studiesHandler(lister))
	mux.HandleFunc("GET /studies/{studyUID}/series", studySeriesHandler(lister))
	mux.HandleFunc("GET /studies/{studyUID}/series/{seriesUID}/instances", seriesInstancesHandler(lister))
	mux.HandleFunc("GET /studies/{studyUID}/series/{seriesUID}/instances/{instanceUID}", acceleratedInstanceHandler(lister, fetcher))
	mux.HandleFunc("POST /studies/{studyUID}/prefetch", prefetchHandler(prefetcher))
	mux.HandleFunc("GET /prefetch/{jobID}", prefetchStatusHandler(prefetcher))

	return mux
}
