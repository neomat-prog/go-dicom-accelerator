// Package main starts the HTTP server for the go-index service.
//
// At the moment, this server provides a basic health check endpoint and acts as
// the application's entry point. It is intended to expand into a DICOM-focused
// service that will first read .dcm files from local storage and later
// integrate with GCP Healthcare API and a DICOM Store.

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/neomat-prog/go-dicom-gateway/dicomfetch"
	"github.com/neomat-prog/go-dicom-gateway/internal/config"
	"github.com/neomat-prog/go-dicom-gateway/internal/httpapi"
	"github.com/neomat-prog/go-dicom-gateway/source"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatal(err)
	}

	src, lister, prober, err := buildSource(cfg)
	if err != nil {
		log.Fatal(err)
	}

	fetcher := dicomfetch.New(src, dicomfetch.Options{
		MaxConcurrency: cfg.MaxConcurrency,
		WindowBehind:   cfg.WindowBehind,
		WindowAhead:    cfg.WindowAhead,
		RequestTimeout: cfg.RequestTimeout,
	})

	if cfg.RunSmokeTest {
		if err := runAcceleratorSmokeTest(ctx, lister, fetcher); err != nil {
			log.Fatal(err)
		}
	}

	prefetcher := httpapi.NewPrefetchManager(lister, fetcher)

	server := &http.Server{
		Addr:    cfg.ServerAddr,
		Handler: httpapi.NewAcceleratedMux(src, cfg.SourceType, prober, lister, fetcher, prefetcher),
	}

	log.Println("Starting server on", cfg.ServerAddr)
	log.Fatal(server.ListenAndServe())

}

func buildSource(cfg config.Config) (source.Source, source.StudyLister, source.Prober, error) {
	switch cfg.SourceType {
	case "local-directory":
		src := source.NewLocalDirectory(cfg.LocalDICOMRoot)
		return src, src, src, nil
	default:
		return nil, nil, nil, fmt.Errorf("unsupported source type %q", cfg.SourceType)
	}
}

func runAcceleratorSmokeTest(ctx context.Context, lister source.StudyLister, fetcher *dicomfetch.Fetcher) error {

	seriesList, err := lister.StudySeries(ctx, "")
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
