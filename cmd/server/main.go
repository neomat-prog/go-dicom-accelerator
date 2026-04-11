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
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	log.Println("server is running on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
