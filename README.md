# DICOM Retrieval Accelerator

[![Go Version](https://img.shields.io/badge/Go-1.26.2-00ADD8?logo=go&logoColor=white)](./go.mod)
[![GCP](https://img.shields.io/badge/GCP-Storage-1d4ed8)](#sources)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

<img align="right" width="200" src="./assets/gopher.png" alt="DICOM Retrieval Accelerator project visual">

A Go library for fast, concurrent DICOM retrieval against multiple backends. It sits between a viewer and remote storage, turning sequential instance requests into parallel background fetches with a server-side sliding window cache.

```text
OHIF Viewer / Go App
        |
        v
DICOM Retrieval Accelerator
        |
        +-- Google Cloud Storage
        +-- Local filesystem
```

## How It Works

DICOM viewers typically request instances one at a time as users scroll through a series. Each request waits for the backend slow when that backend is cloud storage.

This library intercepts that pattern:

```text
Viewer asks for instance N
Library returns instance N
Library concurrently fetches N-3 through N+3 into cache
Next viewer request hits cache instead of the network
```

The expensive retrieval is moved to Go, where bounded concurrency, timeouts, cancellation, and a sliding window cache are applied in a controlled way.

## Core Abstractions

```text
source.Source   — "I know where the DICOM bytes live."
dicomfetch.Fetcher — "I know which nearby instances to fetch and how many at once."
internal/httpapi   — "I translate HTTP requests into Fetcher calls."
```

**`source.Source`** — implemented by each backend adapter. Provides `Instance` (fetch bytes) and `StudyMetadata`. Companion interfaces `StudyLister` and `SeriesLister` expose study/series/instance discovery.

**`dicomfetch.Fetcher`** — wraps a `Source` with bounded-concurrency fetching, sliding-window selection, per-request timeouts, and an in-memory cache keyed by `(StudyUID, SeriesUID, SOPInstanceUID)`.

## Library Usage

```go
import (
    "github.com/neomat-prog/go-dicom-gateway/dicomfetch"
    "github.com/neomat-prog/go-dicom-gateway/source"
)

// Build a source (local or GCS)
src := source.NewLocalDirectory("/path/to/dicom")

// Build a fetcher
fetcher := dicomfetch.New(src, dicomfetch.Options{
    MaxConcurrency: 6,
    WindowBehind:   3,
    WindowAhead:    3,
    RequestTimeout: 30 * time.Second,
})

// List instances in a series
instances, err := src.SeriesInstances(ctx, studyUID, seriesUID)

// Build refs and fetch a window centered on the requested index
refs := make([]source.InstanceRef, len(instances))
for i, info := range instances {
    refs[i] = info.Ref
}

window, err := fetcher.FetchWindow(ctx, refs, requestedIndex)
for _, inst := range window {
    // inst.Data contains the full DICOM bytes
    // Neighbors are already cached for the next request
}
```

## Sources

### Local filesystem

```go
src := source.NewLocalDirectory("/path/to/dicom/root")
```

Walks all `.dcm` files under the root, parses metadata with `dicom.SkipPixelData()`, and groups by Study/Series UID.

### Google Cloud Storage

Files must follow the path convention `{studyUID}/{seriesUID}/{sopUID}.dcm` (optionally under a prefix).

```go
src, err := source.NewGCSSource(ctx, "my-bucket", "studies/")
```

Credentials are read from `GOOGLE_APPLICATION_CREDENTIALS` or `gcloud auth application-default login`.

Use the included upload tool to push local DICOM files to GCS with the correct path structure:

```bash
go run ./cmd/upload --bucket=my-bucket --source=./dicom-dir --prefix=studies/
```

## HTTP Gateway

The included gateway demonstrates the library over HTTP. It exposes DICOMweb-shaped routes and wires them through `dicomfetch.Fetcher`.

### Configuration

Create a `.env` file in the repository root:

```env
# Local filesystem
SOURCE_TYPE=local-directory
LOCAL_DICOM_ROOT=./sample-dicom

# Google Cloud Storage
SOURCE_TYPE=gcs
GCS_BUCKET=my-bucket
GCS_PREFIX=studies/

# Fetcher tuning
FETCH_MAX_CONCURRENCY=6
FETCH_WINDOW_BEHIND=3
FETCH_WINDOW_AHEAD=3
FETCH_REQUEST_TIMEOUT=30s

# Run a window fetch smoke test on startup
RUN_SMOKE_TEST=true
```

### Running

```bash
make run
```

Server listens on `:8081` by default (`SERVER_ADDR` to override).

### Routes

```text
GET  /healthz
GET  /studies/{studyUID}/series
GET  /studies/{studyUID}/series/{seriesUID}/instances
GET  /studies/{studyUID}/series/{seriesUID}/instances/{instanceUID}
POST /studies/{studyUID}/prefetch
GET  /prefetch/{jobID}
```

### Examples

```bash
# Health check
curl http://localhost:8081/healthz

# Fetch a single instance (neighbors are warmed into cache concurrently)
curl -OJ http://localhost:8081/studies/{studyUID}/series/{seriesUID}/instances/{sopUID}

# Start a full-study background prefetch
curl -X POST http://localhost:8081/studies/{studyUID}/prefetch \
  -H 'Content-Type: application/json' \
  -d '{"seriesBatchSize": 6}'

# Check prefetch progress
curl http://localhost:8081/prefetch/prefetch-1
```

Health response:

```json
{"status": "ok", "source": {"type": "gcs", "status": "ok"}, "cache": {"size": 7}}
```

Prefetch status response:

```json
{
  "jobId": "prefetch-1",
  "status": "running",
  "seriesTotal": 4,
  "seriesCompleted": 2,
  "instancesTotal": 389,
  "instancesCompleted": 194,
  "bytesLoaded": 39845888
}
```

## Performance

The point of the accelerator is that the *second* read of a study is served
from memory instead of the backend. `scripts/bench_prefetch.sh` measures it
end-to-end: it prefetches a series cold (streamed from the backend) and then
again warm (served from the in-memory LRU cache).

```bash
# 1. start the server fresh so the cache is empty
FETCH_MAX_CONCURRENCY=32 go run ./cmd/server &

# 2. run the benchmark (auto-discovers the largest series)
scripts/bench_prefetch.sh
```

Sample run against a GCS bucket, one 389-instance series (78 MB):

| Run | Source | Concurrency | Elapsed | Throughput | Per instance |
| --- | --- | --- | --- | --- | --- |
| Cold | GCS | 6 | 52.4 s | 1.5 MB/s | 135 ms |
| Cold | GCS | 32 | 9.5 s | 8.2 MB/s | 24 ms |
| Warm | LRU cache | — | 1.2 s | 66 MB/s | 3 ms |

- **Warm ~40× cold.** Cache working. Ratio is network-independent; treat absolute MB/s as a sample.
- **Cold is round-trip bound.** Scales with `FETCH_MAX_CONCURRENCY` (6 to 32 cut it 5.5×).
- Cache is byte-bounded (`FETCH_MAX_CACHE_BYTES`, default 1 GiB), evicts least-recently-used.

## Development

```bash
make test    # run all tests
make build   # build the server binary
make fmt     # format code
make vet     # run go vet
```

Targeted tests:

```bash
go test ./dicomfetch/...
go test ./source/...
go test ./internal/httpapi/...
go test ./internal/config/...
```
