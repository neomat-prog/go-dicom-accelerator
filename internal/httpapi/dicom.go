package httpapi

import (
	"net/http"
	"os"
)

func dicomHandler(w http.ResponseWriter, r *http.Request) {
	file, err := os.Open("./test.dcm")
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

	w.Header().Set("Content-Type", "application/dicom")
	w.Header().Set("Content-Disposition", `inline; filename="test.dcm"`)

	http.ServeContent(w, r, "test.dcm", info.ModTime(), file)
}
