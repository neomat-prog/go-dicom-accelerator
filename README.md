# DICOM Retrieval Accelerator

[![Go Version](https://img.shields.io/badge/Go-1.26.2-00ADD8?logo=go&logoColor=white)](./go.mod)
[![Status](https://img.shields.io/badge/status-library%20first%20WIP-0f766e)](#status)
[![GCP](https://img.shields.io/badge/GCP-DICOM%20retrieval-1d4ed8)](#target-architecture)

<img align="right" width="200" src="./assets/gopher.png" alt="DICOM Retrieval Accelerator project visual">

### A Go project for fast, concurrent DICOM retrieval, starting with GCP-backed imaging workflows.

This repository is being shaped into a library-first project for downloading
DICOM instances faster by using bounded concurrent requests, streaming,
retry-aware fetching, and source adapters.

The current codebase is still a small HTTP gateway that serves one local DICOM
file and exposes basic metadata. That gateway is useful as a working example,
but the long-term reusable value is the retrieval engine underneath it.

## Project Direction

The intended open-source shape is:

- a public Go library for concurrent DICOM retrieval
- a GCP Healthcare API / DICOM Store adapter
- a local file adapter for development and tests
- an optional OHIF-facing gateway example
- benchmarks for sequential vs concurrent retrieval

In practical terms:

```text
OHIF Viewer / Go App
        |
        v
DICOM Retrieval Accelerator
        |
        +-- GCP Healthcare API / DICOM Store
        +-- Cloud Storage
        +-- local files
        +-- future DICOMweb-compatible sources
```

## Core Idea: Server-Side Sliding Windows

OHIF and other web viewers often request DICOM instances one at a time while
the user scrolls through a series. That is simple for the viewer, but it can be
slow when every browser request has to wait for a remote cloud object or DICOM
Store request.

This project is meant to sit between the viewer and the storage backend:

```text
OHIF asks for instance 40
Gateway returns instance 40
Gateway prefetches 41-56 and keeps 36-39 warm
Next OHIF request is served from the gateway cache instead of cold GCP storage
```

That does not remove every concurrency limit in the system. GCP quotas, backend
network capacity, and the browser-to-gateway connection still matter. The
useful shift is that expensive remote retrieval is moved to Go, where the
library can use bounded concurrency, retries, caching, cancellation, and
sliding-window prefetching in a controlled way.

The library boundary is intentionally customizable:

- `source.Source` adapters know how to fetch from GCP, GCS, PACS, local disk, or
  a custom archive.
- `dicomfetch.Fetcher` is the planned home for the acceleration strategy:
  bounded concurrency, window selection, request timeouts, and caching.
- `internal/httpapi` is only the gateway example that exposes this behavior to
  HTTP clients like OHIF.

The goal is a win-win shape: OHIF can keep its normal instance-by-instance
request model, while the server turns that access pattern into efficient
backend retrieval.

## Why This Exists

Medical image studies often contain many DICOM instances. Fetching those
instances one by one can make viewer startup and study loading painfully slow,
especially when the source is remote cloud storage or a cloud DICOM store.

This project is intended to provide a focused Go library that can:

- fetch many DICOM instances concurrently
- cap concurrency so callers do not overload a backend
- stream data instead of buffering whole studies unnecessarily
- support cancellation with `context.Context`
- retry transient cloud or network failures
- expose useful metadata for study, series, and instance workflows
- plug into GCP first, while keeping the core library source-agnostic

## Status

This is an early foundation project.

Implemented today:

- `GET /healthz`
- `GET /dicom`
- `GET /dicom/metadata`
- local-file-backed DICOM serving
- metadata parsing for Study Instance UID, Series Instance UID, and SOP Instance UID
- recursive local study discovery with study, series, and instance listings
- sliding-window instance acceleration with bounded concurrency and in-memory caching
- explicit full-series batch prefetch jobs for OHIF-style workflows

## Target Architecture

The planned library boundary should look roughly like this:

```go
type InstanceRef struct {
    StudyUID    string
    SeriesUID   string
    InstanceUID string
}

type Source interface {
    FetchInstance(ctx context.Context, ref InstanceRef) (io.ReadCloser, Metadata, error)
}

type Fetcher struct {
    Source      Source
    Concurrency int
    RetryPolicy RetryPolicy
}
```

The HTTP gateway should call this library. It should not be the main product.

The `dicomfetch` package is intentionally minimal right now. A future version
should grow toward this kind of use:

```go
options := dicomfetch.Options{
    MaxConcurrency: 8,
    BatchSize: 32,
    RequestTimeout: 15 * time.Second,
}

fetcher := dicomfetch.Fetcher{
    Source: src,
    Options: options,
}

// orderedSeries must be sorted in the same order the viewer scrolls through it.
window, err := fetcher.FetchWindow(ctx, orderedSeries, requestedIndex)
if err != nil {
    return err
}

for _, resp := range window {
    defer resp.Body.Close()
    // Stream or cache resp.Body.
}
```

For a beginner-friendly mental model:

```text
Source  = "I know where the DICOM bytes live."
Fetcher = "I know which nearby instances to fetch and how many at once."
Gateway = "I translate OHIF/DICOMweb HTTP requests into fetcher calls."
```

Proposed package layout:

```text
cmd/server/                  current demo gateway entrypoint
dicomfetch/                  future public retrieval library
dicomfetch/gcphealthcare/    future GCP Healthcare API adapter
dicomfetch/gcs/              future Cloud Storage adapter
dicomfetch/local/            future local file adapter
internal/config/             gateway configuration
internal/httpapi/            gateway HTTP handlers
assets/                      README assets
```

## Installation

Clone the repository and download dependencies:

```bash
git clone git@github.com:neomat-prog/DICOM-Retrieval-Accelerator.git
cd DICOM-Retrieval-Accelerator
go mod download
```

The current Go module path is:

```text
github.com/neomat-prog/go-dicom-gateway
```

That module path may change if the repository is renamed around the library.

## Running The Current Gateway

Create a `.env` file in the repository root:

```env
DICOM_FILE_PATH=/absolute/path/to/file.dcm
```

Start the server:

```bash
go run ./cmd/server
```

The server listens on:

```text
http://localhost:8081
```

## Current API

The current gateway exposes:

```text
GET /healthz
GET /dicom
GET /dicom/metadata
GET /studies
GET /studies/{studyUID}/series
GET /studies/{studyUID}/series/{seriesUID}/instances
GET /studies/{studyUID}/series/{seriesUID}/instances/{instanceUID}
POST /studies/{studyUID}/prefetch
GET /prefetch/{jobID}
```

Example requests:

```bash
curl http://localhost:8081/healthz
curl http://localhost:8081/dicom/metadata
curl http://localhost:8081/studies
curl http://localhost:8081/studies/{studyUID}/series
curl -OJ http://localhost:8081/studies/{studyUID}/series/{seriesUID}/instances/{sopInstanceUID}
```

Example health response:

```json
{"status":"ok"}
```

Example metadata response:

```json
{
  "studyInstanceUID": "1.2.840....",
  "seriesInstanceUID": "1.2.840....",
  "sopInstanceUID": "1.2.840...."
}
```

## Full-Series Batch Prefetch

The normal instance route still returns one requested DICOM instance. The
prefetch route starts a background job that warms complete series into the same
long-lived in-memory `dicomfetch.Fetcher` cache used by that instance route.

If `seriesInstanceUIDs` is omitted or empty, the gateway prefetches every series
in the study. `seriesBatchSize` defaults to `6` when it is missing or invalid.

Start a full-study prefetch:

```bash
curl -s -X POST http://localhost:8081/studies/{studyUID}/prefetch \
  -H 'Content-Type: application/json' \
  -d '{"seriesBatchSize":6}'
```

Start a selected-series prefetch:

```bash
curl -s -X POST http://localhost:8081/studies/{studyUID}/prefetch \
  -H 'Content-Type: application/json' \
  -d '{"seriesInstanceUIDs":["{seriesUID}"],"seriesBatchSize":6}'
```

Response:

```json
{
  "jobId": "prefetch-1",
  "status": "running",
  "statusUrl": "/prefetch/prefetch-1"
}
```

Check progress:

```bash
curl -s http://localhost:8081/prefetch/prefetch-1
```

Example status:

```json
{
  "jobId": "prefetch-1",
  "studyInstanceUID": "1.2.840....",
  "status": "running",
  "seriesTotal": 16,
  "seriesCompleted": 6,
  "instancesTotal": 670,
  "instancesCompleted": 240,
  "bytesLoaded": 12345678,
  "currentBatch": 2,
  "errors": []
}
```

## Planned Gateway Routes

These routes are still future work for DICOMweb compatibility and richer OHIF
integration:

```text
GET /studies/{studyUID}
```

## Contributing

Contributions are welcome, especially around retrieval APIs, concurrency
behavior, GCP integration, tests, and documentation.

This project should stay small, practical, and library-first: the gateway exists
to prove the retrieval layer works.
