package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/neomat-prog/go-dicom-gateway/source"
)

func dicomMetadataHandler(src source.Source) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		metadata, err := src.StudyMetadata(r.Context(), "")
		if err != nil {
			writeSourceError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(metadata); err != nil {
			http.Error(w, "failed to encode metadata", http.StatusInternalServerError)
			return
		}
	}
}

func dicomHandler(src source.Source) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := src.Instance(r.Context(), source.InstanceRef{})
		if err != nil {
			writeSourceError(w, err)
			return
		}
		defer resp.Body.Close()

		w.Header().Set("Content-Type", resp.ContentType)
		w.Header().Set("Content-Disposition", "inline; filename="+strconv.Quote(resp.Filename))

		if resp.ContentLength >= 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
		}

		if _, err := io.Copy(w, resp.Body); err != nil {
			return
		}
	}
}

func writeSourceError(w http.ResponseWriter, err error) {
	switch {
	case source.IsKind(err, source.ErrorKindBadRequest):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case source.IsKind(err, source.ErrorKindNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case source.IsKind(err, source.ErrorKindNotAcceptable):
		http.Error(w, err.Error(), http.StatusNotAcceptable)
	default:
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}
