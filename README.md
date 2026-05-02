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
- minimal `dicomfetch` package with `Options` and `Fetcher` placeholders

## Not Implemented Yet

| Feature                                   | Description |
|------------------------------------------|-------------|
| Public importable retrieval package      | A reusable Go package for external use |
| Sliding-window fetcher                  | Bounded concurrent prefetch around the requested instance |
| Production study or series downloads     | Disk-backed or stream-oriented fetching of multiple DICOM instances |
| GCP Healthcare API adapter              | Integration with GCP DICOM Store |
| GCS adapter                             | Support for Google Cloud Storage sources |
| DICOMweb route compatibility            | Standard DICOMweb API support |
| Benchmark suite                         | Performance comparisons (sequential vs concurrent) |
| Production OHIF integration             | Full integration with OHIF viewer workflows |

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
http://localhost:8080
```

## Current API

The current gateway exposes:

```text
GET /healthz
GET /dicom
GET /dicom/metadata
```

Example requests:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/dicom/metadata
curl -OJ http://localhost:8080/dicom
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

## Planned Gateway Routes

These routes are not implemented yet, but they match the intended OHIF-facing
gateway direction:

```text
GET /studies
GET /studies/{studyUID}
GET /studies/{studyUID}/series
GET /studies/{studyUID}/series/{seriesUID}/instances
GET /studies/{studyUID}/series/{seriesUID}/instances/{instanceUID}
```

## Roadmap

Near-term work:

- extract retrieval concepts out of `internal/httpapi`
- create a `dicomfetch.SelectWindow` helper
- create a `dicomfetch.FetchInstance` method
- create a `dicomfetch.FetchWindow` method first sequentially, then with bounded concurrency
- add a production-ready cache behind the fetcher
- add local-source tests that do not depend on cloud credentials
- define the GCP Healthcare API adapter boundary
- add retries, cancellation, and timeout behavior
- document expected OHIF request and response contracts

## Learning Path For Building This Project

If you are newer to Go, build this in small layers:

1. Understand `source.Source`.
   This is an interface. Any adapter only needs to satisfy the methods in
   `source/source.go`.

2. Create a `dicomfetch.SelectWindow` helper.
   This should be pure slice math. Given an ordered series and a current index,
   it returns the nearby instances that should be warmed.

3. Create `dicomfetch.FetchWindow` sequentially.
   Before adding goroutines, make it fetch the selected window one instance at a
   time. Simple first, fast second.

4. Add bounded concurrency.
   This is the first concurrency lesson. Use goroutines plus a buffered channel
   as a semaphore so only `MaxConcurrency` backend fetches run at once.

5. Add caching.
   Real DICOM studies can be large, so start with a tiny memory cache for
   learning, then evolve it toward a size-limited memory cache, disk cache, or
   streaming cache.

6. Add a real adapter.
   Start with local files, then implement GCP Healthcare API or GCS behind the
   same `source.Source` interface.

7. Make the gateway OHIF-compatible.
   The HTTP layer should translate OHIF/DICOMweb requests into `InstanceRef`
   values and ordered series lists, then let `dicomfetch` do the acceleration.

Later work:

- implement GCP Healthcare API / DICOM Store fetching
- implement Cloud Storage fetching
- add benchmark results for sequential vs concurrent retrieval
- add structured logging and basic metrics hooks
- add DICOMweb-compatible route helpers where useful

## Contributing

Contributions are welcome, especially around retrieval APIs, concurrency
behavior, GCP integration, tests, and documentation.

This project should stay small, practical, and library-first: the gateway exists
to prove the retrieval layer works.
