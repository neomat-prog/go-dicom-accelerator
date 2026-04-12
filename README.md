# go-dicom-gateway

[![Go Version](https://img.shields.io/badge/Go-1.26.2-00ADD8?logo=go&logoColor=white)](./go.mod)
[![Project Status](https://img.shields.io/badge/status-in%20progress-0f766e)](./AGENT.md)
[![OHIF Gateway](https://img.shields.io/badge/OHIF-DICOM%20gateway-1d4ed8)](./AGENT.md)

### Go HTTP service for serving DICOM instances and metadata today, and growing into an OHIF-facing imaging gateway backed by GCP services

This project provides a simple Go backend for DICOM delivery workflows.

It currently provides:
- a health endpoint for service checks
- a local-file-backed DICOM instance endpoint
- a DICOM metadata endpoint for study, series, and instance UIDs
- a clean starting point for later OHIF, GCS, and Healthcare API integration

The intended flow for this repository is:

`OHIF Viewer -> Go backend -> GCP Healthcare API / Cloud Storage`

Right now the service is intentionally small. The goal is to build the platform in layers: first local retrieval and correct HTTP behavior, then cloud-backed retrieval, then OHIF integration, and only after that performance work such as caching, retries, prefetching, and concurrency-oriented optimizations.

<p align="center">
  <img src="image-Photoroom.png" alt="Go DICOM Gateway project visual" width="260">
</p>

## Installation

Clone the repository and download the Go dependencies:

```bash
git clone git@github.com:neomat-prog/DICOM-Retrieval-Accelerator.git
cd DICOM-Retrieval-Accelerator
go mod download
```

## Usage

Create a `.env` file in the repository root:

```env
DICOM_FILE_PATH=/absolute/path/to/file.dcm
```

Start the server:

```bash
go run ./cmd/server
```

The server listens on `http://localhost:8080`.

## API

The current server exposes the following routes:

```text
GET /healthz
GET /dicom
GET /dicom/metadata
```

### Example requests

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/dicom/metadata
curl -OJ http://localhost:8080/dicom
```

### Example responses

`GET /healthz`

```json
{"status":"ok"}
```

`GET /dicom/metadata`

```json
{
  "studyInstanceUID": "1.2.840....",
  "seriesInstanceUID": "1.2.840....",
  "sopInstanceUID": "1.2.840...."
}
```

## What It Does Today

The codebase currently includes:

- `cmd/server/main.go` as the application entrypoint
- `internal/config/config.go` for loading configuration from `.env`
- `internal/httpapi/routes.go` for route registration
- `internal/httpapi/dicom.go` for serving the configured DICOM file
- `internal/httpapi/dicom_metadata.go` for parsing and returning core DICOM identifiers

This is a foundation project, not the final architecture.

## Target Architecture

The practical target shape for the project is:

1. OHIF sends requests to this Go service.
2. The service validates requests and formats responses.
3. The service fetches DICOM data from a backing source.
4. The backing source starts as local disk, then moves to GCS or GCP Healthcare API / DICOM Store.
5. The service returns data in a format OHIF can consume.

## Likely Future Endpoints

These routes are not implemented yet, but they match the direction already outlined in the code and project notes:

```text
GET /studies
GET /studies/{studyUID}
GET /studies/{studyUID}/series
GET /studies/{studyUID}/series/{seriesUID}/instances
GET /studies/{studyUID}/series/{seriesUID}/instances/{instanceUID}
```

## Development Roadmap

### Phase 1: Stabilize the HTTP Foundation

Focus on:

- `net/http`
- handlers
- routing
- correct status codes
- request and response behavior
- serving files correctly

### Phase 2: Return One DICOM From a Real Source

Start with:

- local disk
- or a single GCS object

### Phase 3: Add GCP Integration

Planned areas:

- GCP project setup
- service account authentication
- Healthcare API integration
- DICOM Store retrieval

### Phase 4: Connect OHIF

At this stage the backend becomes the integration layer that reveals:

- what OHIF expects from the backend
- which routes and metadata shapes matter
- where latency and bottlenecks appear

### Phase 5: Add Go Performance Value

Only after the basics work:

- goroutines
- worker pools
- retries
- caching
- prefetching
- prioritized fetching
- connection reuse

## Repository Layout

```text
cmd/server/             application entrypoint
internal/config/        environment and config loading
internal/httpapi/       HTTP handlers and routing
docs/assets/            README visuals
```

## Notes

- The service currently serves a single configured DICOM file from local disk.
- GCP Healthcare API and Cloud Storage integration are planned, not implemented yet.
- The current engineering direction favors small handlers, explicit routing, and readable data flow over early abstractions.

## Contribute

Contributions are welcome. Good near-term improvements include:

- refining route structure
- improving handler behavior and validation
- adding request parameters for file selection
- introducing retrieval service layers
- preparing the codebase for GCP-backed fetches
- documenting the OHIF-facing contract
