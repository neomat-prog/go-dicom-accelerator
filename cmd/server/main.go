// Package main starts the HTTP server for the go-index service.
//
// At the moment, this server provides a basic health check endpoint and acts as
// the application's entry point. It is intended to expand into a DICOM-focused
// service that will first read .dcm files from local storage and later
// integrate with GCP Healthcare API and a DICOM Store.

package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/neomat-prog/go-dicom-gateway/dicomfetch"
	"github.com/neomat-prog/go-dicom-gateway/internal/httpapi"
	"github.com/neomat-prog/go-dicom-gateway/source"
)

const serverAddr = ":8081"

func main() {
	ctx := context.Background()

	src := source.NewLocalDirectory("sample-dicom/S1241164704480_images")

	options := dicomfetch.DefaultOptions()
	options.MaxConcurrency = 6
	options.WindowBehind = 3
	options.WindowAhead = 3
	options.RequestTimeout = 30 * time.Second

	fetcher := dicomfetch.New(src, options)

	if err := runAcceleratorSmokeTest(ctx, src, fetcher); err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:    serverAddr,
		Handler: httpapi.NewAcceleratedMux(src, src, fetcher),
	}

	log.Println("Starting server on", serverAddr)
	log.Fatal(server.ListenAndServe())
}

func runAcceleratorSmokeTest(ctx context.Context, src *source.LocalDirectorySource, fetcher *dicomfetch.Fetcher) error {

	seriesList, err := src.StudySeries(ctx, "")
	if err != nil {
		return err
	}

	largest := seriesList[0]
	for _, series := range seriesList[1:] {
		if len(series.Instances) > len(largest.Instances) {
			largest = series
		}
	}

	instances := largest.Instances

	refs := make([]source.InstanceRef, len(instances))
	for i, info := range instances {
		refs[i] = info.Ref
	}

	center := len(refs) / 2

	window, err := fetcher.FetchWindow(ctx, refs, center)
	if err != nil {
		return err
	}

	totalBytes := 0
	for _, instance := range window {
		totalBytes += len(instance.Data)
	}

	log.Printf("Study discovery: series count=%d", len(seriesList))
	log.Printf("Study discovery: largest series=%s instances=%d", largest.SeriesInstanceUID, len(instances))
	log.Printf("Accelerator smoke test: center index=%d sop=%s", center, refs[center].SOPInstanceUID)
	log.Printf("Accelerator smoke test: fetched window=%d instances bytes=%d", len(window), totalBytes)

	return nil
}
