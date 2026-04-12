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

	"github.com/neomat-prog/go-dicom-gateway/internal/config"
	"github.com/neomat-prog/go-dicom-gateway/internal/httpapi"
)

func main() {
	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:    ":8080",
		Handler: httpapi.NewMux(cfg.DICOMFilePath),
	}

	log.Println("Starting server on :8080")
	log.Fatal(server.ListenAndServe())
}
