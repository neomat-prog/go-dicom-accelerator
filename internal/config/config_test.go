package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad_EnvironmentOverridesDotEnv(t *testing.T) {
	clearConfigEnv(t)

	root := t.TempDir()
	envPath := filepath.Join(root, ".env")

	writeFile(t, envPath, "SOURCE_TYPE=gcs\nGCS_BUCKET=from-dotenv\nSERVER_ADDR=:8081\n")
	t.Setenv(gcsBucketKey, "override-bucket")
	t.Setenv(serverAddrKey, ":9090")

	cfg, err := Load(envPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.GCSBucket != "override-bucket" {
		t.Fatalf("want GCSBucket %q, got %q", "override-bucket", cfg.GCSBucket)
	}
	if cfg.ServerAddr != ":9090" {
		t.Fatalf("want server addr :9090, got %q", cfg.ServerAddr)
	}
}

func TestLoad_ParsesFetchOptionsAndSmokeTest(t *testing.T) {
	clearConfigEnv(t)

	envPath := filepath.Join(t.TempDir(), ".env")
	writeFile(t, envPath, `
SERVER_ADDR=:9091
SOURCE_TYPE=gcs
GCS_BUCKET=my-bucket
FETCH_MAX_CONCURRENCY=12
FETCH_WINDOW_BEHIND=5
FETCH_WINDOW_AHEAD=9
FETCH_REQUEST_TIMEOUT=45s
FETCH_MAX_CACHE_BYTES=2048
RUN_SMOKE_TEST=true
`)

	cfg, err := Load(envPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.ServerAddr != ":9091" {
		t.Fatalf("want server addr :9091, got %q", cfg.ServerAddr)
	}
	if cfg.MaxConcurrency != 12 {
		t.Fatalf("want max concurrency 12, got %d", cfg.MaxConcurrency)
	}
	if cfg.WindowBehind != 5 {
		t.Fatalf("want window behind 5, got %d", cfg.WindowBehind)
	}
	if cfg.WindowAhead != 9 {
		t.Fatalf("want window ahead 9, got %d", cfg.WindowAhead)
	}
	if cfg.RequestTimeout != 45*time.Second {
		t.Fatalf("want request timeout 45s, got %s", cfg.RequestTimeout)
	}
	if cfg.MaxCacheBytes != 2048 {
		t.Fatalf("want max cache bytes 2048, got %d", cfg.MaxCacheBytes)
	}
	if !cfg.RunSmokeTest {
		t.Fatalf("want smoke test enabled")
	}
}

func TestLoad_GCSSourceRequiresBucket(t *testing.T) {
	clearConfigEnv(t)

	envPath := filepath.Join(t.TempDir(), ".env")
	writeFile(t, envPath, "SOURCE_TYPE=gcs\n")

	_, err := Load(envPath)
	if err == nil {
		t.Fatal("expected error when GCS_BUCKET is missing, got nil")
	}
	if !strings.Contains(err.Error(), "GCS_BUCKET") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_GCSSourceReadsConfig(t *testing.T) {
	clearConfigEnv(t)

	envPath := filepath.Join(t.TempDir(), ".env")
	writeFile(t, envPath, "SOURCE_TYPE=gcs\nGCS_BUCKET=my-bucket\nGCS_PREFIX=studies/\n")

	cfg, err := Load(envPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.GCSBucket != "my-bucket" {
		t.Fatalf("want GCSBucket %q, got %q", "my-bucket", cfg.GCSBucket)
	}
	if cfg.GCSPrefix != "studies/" {
		t.Fatalf("want GCSPrefix %q, got %q", "studies/", cfg.GCSPrefix)
	}
}

func TestReadDotEnv_ParsesCommentsExportAndQuotes(t *testing.T) {
	root := t.TempDir()
	envPath := filepath.Join(root, ".env")

	writeFile(t, envPath, `
# comment
export GCS_BUCKET = "my-bucket"
OTHER_KEY = 'value'
`)

	values, envDir, err := readDotEnv(envPath)
	if err != nil {
		t.Fatalf("readDotEnv returned error: %v", err)
	}

	if envDir != root {
		t.Fatalf("want envDir %q, got %q", root, envDir)
	}

	if values[gcsBucketKey] != "my-bucket" {
		t.Fatalf("want GCS_BUCKET %q, got %q", "my-bucket", values[gcsBucketKey])
	}

	if values["OTHER_KEY"] != "value" {
		t.Fatalf("want OTHER_KEY %q, got %q", "value", values["OTHER_KEY"])
	}
}

func TestReadDotEnv_ReturnsErrorForInvalidLine(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	writeFile(t, envPath, "THIS_IS_NOT_VALID\n")

	_, _, err := readDotEnv(envPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "invalid .env line") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		serverAddrKey,
		sourceTypeKey,
		gcsBucketKey,
		gcsPrefixKey,
		maxConcurrencyKey,
		windowBehindKey,
		windowAheadKey,
		requestTimeoutKey,
		runSmokeTestKey,
	} {
		t.Setenv(key, "")
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte(strings.TrimLeft(contents, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
