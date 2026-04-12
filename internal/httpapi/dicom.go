package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
)

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
