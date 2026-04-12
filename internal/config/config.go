package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	DICOMFilePath string
}

func Load(envPath string) (Config, error) {
	if err := loadDotEnvFile(envPath); err != nil {
		return Config{}, err
	}

	dicomFilePath, ok := os.LookupEnv("DICOM_FILE_PATH")
	if !ok || strings.TrimSpace(dicomFilePath) == "" {
		return Config{}, fmt.Errorf("DICOM_FILE_PATH is not set")
	}

	return Config{
		DICOMFilePath: dicomFilePath,
	}, nil
}

func loadDotEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid .env line: %q", line)
		}

		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			return fmt.Errorf("invalid .env line: %q", line)
		}

		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set env %s: %w", key, err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	return nil
}
