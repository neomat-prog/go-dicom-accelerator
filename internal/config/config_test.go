package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad_UsesDefaultDotEnvAndResolvesRelativeLocalDICOMRoot(t *testing.T) {
	clearConfigEnv(t)

	root := t.TempDir()
	dicomRoot := filepath.Join(root, "data")
	if err := os.MkdirAll(dicomRoot, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dicomRoot, err)
	}
	writeFile(t, filepath.Join(root, ".env"), "LOCAL_DICOM_ROOT=data\n")

	runDir := filepath.Join(root, "cmd", "server")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", runDir, err)
	}
	chdir(t, runDir)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	want := filepath.Clean(dicomRoot)
	if !samePath(t, cfg.LocalDICOMRoot, want) {
		t.Fatalf("want %q, got %q", want, cfg.LocalDICOMRoot)
	}
	if cfg.ServerAddr != defaultServerAddr {
		t.Fatalf("want default server addr %q, got %q", defaultServerAddr, cfg.ServerAddr)
	}
	if cfg.SourceType != defaultSourceType {
		t.Fatalf("want default source type %q, got %q", defaultSourceType, cfg.SourceType)
	}
}

func TestLoad_EnvironmentOverridesDotEnv(t *testing.T) {
	clearConfigEnv(t)

	root := t.TempDir()

	envPath := filepath.Join(root, ".env")
	fromDotEnv := filepath.Join(root, "from-dotenv")
	overridePath := filepath.Join(root, "override")
	if err := os.MkdirAll(fromDotEnv, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", fromDotEnv, err)
	}
	if err := os.MkdirAll(overridePath, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", overridePath, err)
	}

	writeFile(t, envPath, "LOCAL_DICOM_ROOT=from-dotenv\nSERVER_ADDR=:8081\n")
	t.Setenv(localDICOMRootKey, overridePath)
	t.Setenv(serverAddrKey, ":9090")

	cfg, err := Load(envPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	want := filepath.Clean(overridePath)
	if cfg.LocalDICOMRoot != want {
		t.Fatalf("want %q, got %q", want, cfg.LocalDICOMRoot)
	}
	if cfg.ServerAddr != ":9090" {
		t.Fatalf("want server addr :9090, got %q", cfg.ServerAddr)
	}
}

func TestLoad_ParsesFetchOptionsAndSmokeTest(t *testing.T) {
	clearConfigEnv(t)

	root := t.TempDir()
	dicomRoot := filepath.Join(root, "dicom")
	if err := os.MkdirAll(dicomRoot, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dicomRoot, err)
	}

	envPath := filepath.Join(root, ".env")
	writeFile(t, envPath, `
SERVER_ADDR=:9091
SOURCE_TYPE=local-directory
LOCAL_DICOM_ROOT=dicom
FETCH_MAX_CONCURRENCY=12
FETCH_WINDOW_BEHIND=5
FETCH_WINDOW_AHEAD=9
FETCH_REQUEST_TIMEOUT=45s
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
	if !cfg.RunSmokeTest {
		t.Fatalf("want smoke test enabled")
	}
}

func TestLoad_ReturnsMissingErrorWhenLocalDICOMRootIsNotConfigured(t *testing.T) {
	clearConfigEnv(t)

	missingEnvPath := filepath.Join(t.TempDir(), ".env")

	_, err := Load(missingEnvPath)
	if !errors.Is(err, ErrMissingLocalDICOMRoot) {
		t.Fatalf("want ErrMissingLocalDICOMRoot, got %v", err)
	}
}

func TestLoad_ReturnsErrorWhenLocalDICOMRootPointsToFile(t *testing.T) {
	clearConfigEnv(t)

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "data"), "not a directory")

	envPath := filepath.Join(root, ".env")
	writeFile(t, envPath, "LOCAL_DICOM_ROOT=data\n")

	_, err := Load(envPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "must point to a directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadDotEnv_ParsesCommentsExportAndQuotes(t *testing.T) {
	root := t.TempDir()
	envPath := filepath.Join(root, ".env")

	writeFile(t, envPath, `
# comment
export LOCAL_DICOM_ROOT = "data"
OTHER_KEY = 'value'
`)

	values, envDir, err := readDotEnv(envPath)
	if err != nil {
		t.Fatalf("readDotEnv returned error: %v", err)
	}

	if envDir != root {
		t.Fatalf("want envDir %q, got %q", root, envDir)
	}

	if values[localDICOMRootKey] != "data" {
		t.Fatalf("want LOCAL_DICOM_ROOT %q, got %q", "data", values[localDICOMRootKey])
	}

	if values["OTHER_KEY"] != "value" {
		t.Fatalf("want OTHER_KEY %q, got %q", "value", values["OTHER_KEY"])
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		serverAddrKey,
		sourceTypeKey,
		localDICOMRootKey,
		maxConcurrencyKey,
		windowBehindKey,
		windowAheadKey,
		requestTimeoutKey,
		runSmokeTestKey,
	} {
		t.Setenv(key, "")
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

func chdir(t *testing.T, dir string) {
	t.Helper()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}

	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
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

func samePath(t *testing.T, got, want string) bool {
	t.Helper()

	gotPath, gotErr := filepath.EvalSymlinks(got)
	if gotErr != nil {
		t.Fatalf("resolve %s: %v", got, gotErr)
	}

	wantPath, wantErr := filepath.EvalSymlinks(want)
	if wantErr != nil {
		t.Fatalf("resolve %s: %v", want, wantErr)
	}

	return filepath.Clean(gotPath) == filepath.Clean(wantPath)
}
