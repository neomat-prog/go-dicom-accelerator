// Package main starts the HTTP server for the go-index service.
//
// At the moment, this server provides a basic health check endpoint and acts as
// the application's entry point. It is intended to expand into a DICOM-focused
// service that will first read .dcm files from local storage and later
// integrate with GCP Healthcare API and a DICOM Store.

package main

import (
	"log"
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

func main() {
	http.HandleFunc("/dicom", dicomHandler)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	log.Println("server is running on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
