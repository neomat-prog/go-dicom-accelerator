package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultEnvFile   = ".env"
	dicomFilePathKey = "DICOM_FILE_PATH"
)

var ErrMissingDICOMFilePath = errors.New("DICOM_FILE_PATH is required")

type Config struct {
	DICOMFilePath string
}

func Load(envPath string) (Config, error) {
	if strings.TrimSpace(envPath) == "" {
		envPath = defaultEnvFile
	}

	fileValues, envDir, err := readDotEnv(envPath)
	if err != nil {
		return Config{}, err
	}

	rawPath := strings.TrimSpace(os.Getenv(dicomFilePathKey))
	fromDotEnv := false

	if rawPath == "" {
		rawPath = strings.TrimSpace(fileValues[dicomFilePathKey])
		fromDotEnv = true
	}

	if rawPath == "" {
		return Config{}, ErrMissingDICOMFilePath
	}

	cfg := Config{
		DICOMFilePath: normalizePath(rawPath, envDir, fromDotEnv),
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.DICOMFilePath) == "" {
		return ErrMissingDICOMFilePath
	}

	info, err := os.Stat(c.DICOMFilePath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", c.DICOMFilePath, err)
	}

	if info.IsDir() {
		return fmt.Errorf("%s must point to a file, not a directory", dicomFilePathKey)
	}

	return nil
}

func readDotEnv(path string) (map[string]string, string, error) {
	resolvedPath, exists, err := resolveDotEnvPath(path)
	if err != nil {
		return nil, "", err
	}
	if !exists {
		return map[string]string{}, "", nil
	}

	file, err := os.Open(resolvedPath)
	if err != nil {
		return nil, "", fmt.Errorf("open %s: %w", resolvedPath, err)
	}
	defer file.Close()

	values := make(map[string]string)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, "", fmt.Errorf("invalid .env line: %q", line)
		}

		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		if key == "" {
			return nil, "", fmt.Errorf("invalid .env line: %q", line)
		}

		value = strings.Trim(strings.TrimSpace(value), `"'`)
		values[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, "", fmt.Errorf("read %s: %w", resolvedPath, err)
	}

	return values, filepath.Dir(resolvedPath), nil
}

func normalizePath(path, baseDir string, fromDotEnv bool) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	if fromDotEnv && baseDir != "" {
		return filepath.Clean(filepath.Join(baseDir, path))
	}

	if wd, err := os.Getwd(); err == nil {
		return filepath.Clean(filepath.Join(wd, path))
	}

	return filepath.Clean(path)
}

func resolveDotEnvPath(path string) (string, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false, nil
	}

	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err == nil {
			return filepath.Clean(path), true, nil
		} else if os.IsNotExist(err) {
			return "", false, nil
		} else {
			return "", false, fmt.Errorf("stat %s: %w", path, err)
		}
	}

	if filepath.Base(path) != path {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", false, fmt.Errorf("resolve %s: %w", path, err)
		}

		if _, err := os.Stat(abs); err == nil {
			return abs, true, nil
		} else if os.IsNotExist(err) {
			return "", false, nil
		} else {
			return "", false, fmt.Errorf("stat %s: %w", abs, err)
		}
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", false, fmt.Errorf("getwd: %w", err)
	}

	for {
		candidate := filepath.Join(dir, path)

		if _, err := os.Stat(candidate); err == nil {
			abs, absErr := filepath.Abs(candidate)
			if absErr != nil {
				return "", false, fmt.Errorf("resolve %s: %w", candidate, absErr)
			}
			return abs, true, nil
		} else if !os.IsNotExist(err) {
			return "", false, fmt.Errorf("stat %s: %w", candidate, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", false, nil
}

