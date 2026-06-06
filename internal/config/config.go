package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultEnvFile = ".env"

	serverAddrKey         = "SERVER_ADDR"
	sourceTypeKey         = "SOURCE_TYPE"
	gcsBucketKey          = "GCS_BUCKET"
	gcsPrefixKey          = "GCS_PREFIX"
	maxConcurrencyKey     = "FETCH_MAX_CONCURRENCY"
	windowBehindKey       = "FETCH_WINDOW_BEHIND"
	windowAheadKey        = "FETCH_WINDOW_AHEAD"
	requestTimeoutKey     = "FETCH_REQUEST_TIMEOUT"
	maxCacheBytesKey      = "FETCH_MAX_CACHE_BYTES"
	runSmokeTestKey       = "RUN_SMOKE_TEST"
	defaultServerAddr     = ":8081"
	defaultSourceType     = "gcs"
	defaultMaxConcurrency = 6
	defaultWindowBehind   = 3
	defaultWindowAhead    = 3
	defaultRequestTimeout = 30 * time.Second
	defaultMaxCacheBytes  = 1 << 30 // 1 GiB
	sourceTypeGCS         = "gcs"
)

type Config struct {
	ServerAddr string
	SourceType string
	GCSBucket  string
	GCSPrefix  string

	MaxConcurrency int
	WindowBehind   int
	WindowAhead    int
	RequestTimeout time.Duration
	MaxCacheBytes  int64

	RunSmokeTest bool
}

func Load(envPath string) (Config, error) {
	if strings.TrimSpace(envPath) == "" {
		envPath = defaultEnvFile
	}

	fileValues, _, err := readDotEnv(envPath)
	if err != nil {
		return Config{}, err
	}

	serverAddr := configString(fileValues, serverAddrKey, defaultServerAddr)
	sourceType := configString(fileValues, sourceTypeKey, defaultSourceType)

	gcsBucket := configString(fileValues, gcsBucketKey, "")
	gcsPrefix := configString(fileValues, gcsPrefixKey, "")

	maxConcurrency, err := configInt(fileValues, maxConcurrencyKey, defaultMaxConcurrency)
	if err != nil {
		return Config{}, err
	}

	windowBehind, err := configInt(fileValues, windowBehindKey, defaultWindowBehind)
	if err != nil {
		return Config{}, err
	}

	windowAhead, err := configInt(fileValues, windowAheadKey, defaultWindowAhead)
	if err != nil {
		return Config{}, err
	}

	requestTimeout, err := configDuration(fileValues, requestTimeoutKey, defaultRequestTimeout)
	if err != nil {
		return Config{}, err
	}

	maxCacheBytes, err := configInt64(fileValues, maxCacheBytesKey, defaultMaxCacheBytes)
	if err != nil {
		return Config{}, err
	}

	runSmokeTest, err := configBool(fileValues, runSmokeTestKey, false)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		ServerAddr:     serverAddr,
		SourceType:     sourceType,
		GCSBucket:      gcsBucket,
		GCSPrefix:      gcsPrefix,
		MaxConcurrency: maxConcurrency,
		WindowBehind:   windowBehind,
		WindowAhead:    windowAhead,
		RequestTimeout: requestTimeout,
		MaxCacheBytes:  maxCacheBytes,
		RunSmokeTest:   runSmokeTest,
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Validate checks that the configured source and fetch options are usable.
func (c Config) Validate() error {
	if strings.TrimSpace(c.ServerAddr) == "" {
		return fmt.Errorf("%s is required", serverAddrKey)
	}

	switch c.SourceType {
	case sourceTypeGCS:
		if strings.TrimSpace(c.GCSBucket) == "" {
			return fmt.Errorf("GCS_BUCKET is required for gcs source")
		}
	default:
		return fmt.Errorf("unsupported %s %q", sourceTypeKey, c.SourceType)
	}

	if c.MaxConcurrency <= 0 {
		return fmt.Errorf("%s must be greater than zero", maxConcurrencyKey)
	}
	if c.WindowBehind < 0 {
		return fmt.Errorf("%s cannot be negative", windowBehindKey)
	}
	if c.WindowAhead < 0 {
		return fmt.Errorf("%s cannot be negative", windowAheadKey)
	}
	if c.RequestTimeout < 0 {
		return fmt.Errorf("%s cannot be negative", requestTimeoutKey)
	}
	if c.MaxCacheBytes < 0 {
		return fmt.Errorf("%s cannot be negative", maxCacheBytesKey)
	}

	return nil
}

func configString(fileValues map[string]string, key string, fallback string) string {
	value, _ := configValue(fileValues, key)
	if value == "" {
		return fallback
	}
	return value
}

func configInt(fileValues map[string]string, key string, fallback int) (int, error) {
	raw, _ := configValue(fileValues, key)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return value, nil
}

func configInt64(fileValues map[string]string, key string, fallback int64) (int64, error) {
	raw, _ := configValue(fileValues, key)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return value, nil
}

func configDuration(fileValues map[string]string, key string, fallback time.Duration) (time.Duration, error) {
	raw, _ := configValue(fileValues, key)
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration like 30s: %w", key, err)
	}
	return value, nil
}

func configBool(fileValues map[string]string, key string, fallback bool) (bool, error) {
	raw, _ := configValue(fileValues, key)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return value, nil
}

func configValue(fileValues map[string]string, key string) (string, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw != "" {
		return raw, false
	}
	return strings.TrimSpace(fileValues[key]), true
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
