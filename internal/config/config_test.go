package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_UsesDefaultDotEnvAndResolvesRelativePath(t *testing.T) {
	t.Setenv(dicomFilePathKey, "")

	root := t.TempDir()
	dicomPath := filepath.Join(root, "data", "test.dcm")
	writeFile(t, dicomPath, "dicom")
	writeFile(t, filepath.Join(root, ".env"), "DICOM_FILE_PATH=data/test.dcm\n")

	runDir := filepath.Join(root, "cmd", "server")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", runDir, err)
	}
	chdir(t, runDir)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	want := filepath.Clean(dicomPath)
	if cfg.DICOMFilePath != want {
		t.Fatalf("want %q, got %q", want, cfg.DICOMFilePath)
	}
}

func TestLoad_EnvironmentOverridesDotEnv(t *testing.T) {
	root := t.TempDir()

	envPath := filepath.Join(root, ".env")
	writeFile(t, envPath, "DICOM_FILE_PATH=from-dotenv.dcm\n")

	overridePath := filepath.Join(root, "override.dcm")
	writeFile(t, overridePath, "dicom")
	t.Setenv(dicomFilePathKey, overridePath)

	cfg, err := Load(envPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	want := filepath.Clean(overridePath)
	if cfg.DICOMFilePath != want {
		t.Fatalf("want %q, got %q", want, cfg.DICOMFilePath)
	}
}

func TestLoad_ReturnsMissingErrorWhenDICOMFilePathIsNotConfigured(t *testing.T) {
	t.Setenv(dicomFilePathKey, "")

	missingEnvPath := filepath.Join(t.TempDir(), ".env")

	_, err := Load(missingEnvPath)
	if !errors.Is(err, ErrMissingDICOMFilePath) {
		t.Fatalf("want ErrMissingDICOMFilePath, got %v", err)
	}
}

func TestLoad_ReturnsErrorWhenPathPointsToDirectory(t *testing.T) {
	t.Setenv(dicomFilePathKey, "")

	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dataDir, err)
	}

	envPath := filepath.Join(root, ".env")
	writeFile(t, envPath, "DICOM_FILE_PATH=data\n")

	_, err := Load(envPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "must point to a file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadDotEnv_ParsesCommentsExportAndQuotes(t *testing.T) {
	root := t.TempDir()
	envPath := filepath.Join(root, ".env")

	writeFile(t, envPath, `
# comment
export DICOM_FILE_PATH = "data/test.dcm"
OTHER_KEY = 'value'
`)

	values, envDir, err := readDotEnv(envPath)
	if err != nil {
		t.Fatalf("readDotEnv returned error: %v", err)
	}

	if envDir != root {
		t.Fatalf("want envDir %q, got %q", root, envDir)
	}

	if values[dicomFilePathKey] != "data/test.dcm" {
		t.Fatalf("want DICOM_FILE_PATH %q, got %q", "data/test.dcm", values[dicomFilePathKey])
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
