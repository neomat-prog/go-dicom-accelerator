package httpapi

import (
	"net/http"

	"github.com/neomat-prog/go-dicom-gateway/dicomfetch"
	"github.com/neomat-prog/go-dicom-gateway/source"
)

// NewMux registers the HTTP routes exposed by the application.
func NewMux(src source.Source, sourceType string, prober source.Prober, fetcher *dicomfetch.Fetcher) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(sourceType, prober, fetcher))
	mux.HandleFunc("GET /dicom", dicomHandler(src))
	mux.HandleFunc("GET /dicom/metadata", dicomMetadataHandler(src))
	return mux
}

func NewAcceleratedMux(src source.Source, sourceType string, prober source.Prober, lister source.StudyLister, fetcher *dicomfetch.Fetcher, prefetcher *PrefetchManager) *http.ServeMux {
	mux := NewMux(src, sourceType, prober, fetcher)

	mux.HandleFunc("GET /studies", studiesHandler(lister))
	mux.HandleFunc("GET /studies/{studyUID}/series", studySeriesHandler(lister))
	mux.HandleFunc("GET /studies/{studyUID}/series/{seriesUID}/instances", seriesInstancesHandler(lister))
	mux.HandleFunc("GET /studies/{studyUID}/series/{seriesUID}/instances/{instanceUID}", acceleratedInstanceHandler(lister, fetcher))
	mux.HandleFunc("POST /studies/{studyUID}/prefetch", prefetchHandler(prefetcher))
	mux.HandleFunc("GET /prefetch/{jobID}", prefetchStatusHandler(prefetcher))
	mux.HandleFunc("DELETE /prefetch/{jobID}", prefetchDeleteHandler(prefetcher))

	return mux
}
