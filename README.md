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

## Not Implemented Yet

| Feature                                   | Description |
|------------------------------------------|-------------|
| Public importable retrieval package      | A reusable Go package for external use |
| Concurrent study or series downloads     | Parallel fetching of multiple DICOM instances |
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
- introduce a public `dicomfetch` package
- add a bounded concurrent fetcher
- add local-source tests that do not depend on cloud credentials
- define the GCP Healthcare API adapter boundary
- add retries, cancellation, and timeout behavior
- document expected OHIF request and response contracts

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
