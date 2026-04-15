// This file defines a HTTP handler for serving DICOM files.
// Requests to the /dicom endpoint will return the DICOM file with the appropriate content
// type and disposition headers. The file is read from the path specified in the configuration.

package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// TODO(neomat-prog) 12.04.26: Create a proper endpoint for fetching and serving DICOM files.
// add more endpoints for listing available DICOM files for now and fetching via STUDY UID, SERIES UID, and INSTANCE UID.
// FUTURE: implement a more robust solution to obtain files from DICOM Store GCP

//TODO(neomat-prog) 12.04.26 : detect and expose study/series/instance UIDs from DICOM metadata

/*

GET /studies
GET /studies/{studyUID}
GET /studies/{studyUID}/series
GET /studies/{studyUID}/series/{seriesUID}/instances
GET /studies/{studyUID}/series/{seriesUID}/instances/{instanceUID}

FUTURE: implement a retrieval layer
a metadata/index layer
handlers that call those layers

*/

type DICOMMetadata struct {
	StudyInstanceUID  string `json:"studyInstanceUID"`
	SeriesInstanceUID string `json:"seriesInstanceUID"`
	SOPInstanceUID    string `json:"sopInstanceUID"`
}

func dicomMetadataHandler(dicomFilePath string) http.HandlerFunc {

	// returns metadata from the DICOM file
	return func(w http.ResponseWriter, r *http.Request) {
		metadata, err := readDicomMetadata(dicomFilePath)
		// an error has occurred
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(metadata); err != nil {
			http.Error(w, "failed to encode metadata", http.StatusInternalServerError)
			return
		}
	}
}

func dicomHandler(dicomFilePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		file, err := os.Open(dicomFilePath)
		if err != nil {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil {
			http.Error(w, "could not get file info", http.StatusInternalServerError)
			return
		}

		filename := filepath.Base(dicomFilePath)

		w.Header().Set("Content-Type", "application/dicom")
		w.Header().Set("Content-Disposition", `inline; filename="`+filename+`"`)

		http.ServeContent(w, r, filename, info.ModTime(), file)
	}
}

