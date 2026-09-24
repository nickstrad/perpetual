package config

import (
	"cmp"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL, ListenAddress, RuntimeDir                         string
	MaxRegistrations                                               uint64
	MaxConnections, MaxJobs, QueueSize, Workers, PoolConnections   int
	LockTimeout, StatementTimeout, OperationTimeout, ShutdownGrace time.Duration
}

// Parse reads a supplied environment lookup without accessing the process
// environment, so callers can validate configuration before acquiring resources.
func Parse(getenv func(string) string) (Config, error) {
	if getenv == nil {
		return Config{}, fmt.Errorf("environment lookup is required")
	}
	config := Config{
		DatabaseURL:      getenv("PERPETUAL_DATABASE_URL"),
		ListenAddress:    cmp.Or(getenv("PERPETUAL_LISTEN_ADDRESS"), "127.0.0.1:7777"),
		RuntimeDir:       cmp.Or(getenv("PERPETUAL_RUNTIME_DIR"), "/run/perpetual"),
		LockTimeout:      time.Second,
		StatementTimeout: 5 * time.Second,
		OperationTimeout: 8 * time.Second,
		ShutdownGrace:    10 * time.Second,
	}
	if config.DatabaseURL == "" {
		return Config{}, fmt.Errorf("PERPETUAL_DATABASE_URL is required")
	}
	if err := validateDatabaseURL(config.DatabaseURL); err != nil {
		return Config{}, err
	}
	if err := validateListenAddress(config.ListenAddress); err != nil {
		return Config{}, err
	}
	if !filepath.IsAbs(config.RuntimeDir) {
		return Config{}, fmt.Errorf("PERPETUAL_RUNTIME_DIR must be an absolute path")
	}

	var err error
	// Each ceiling is both the default and the maximum: the environment may
	// only lower these bounds.
	if config.MaxRegistrations, err = parseCount(getenv, "PERPETUAL_MAX_REGISTRATIONS", 1024); err != nil {
		return Config{}, err
	}
	counts := []struct {
		key     string
		ceiling int
		target  *int
	}{
		{"PERPETUAL_MAX_CONNECTIONS", 128, &config.MaxConnections},
		{"PERPETUAL_MAX_JOBS", 128, &config.MaxJobs},
		{"PERPETUAL_QUEUE_SIZE", 128, &config.QueueSize},
		{"PERPETUAL_WORKERS", 4, &config.Workers},
		{"PERPETUAL_POOL_CONNECTIONS", 8, &config.PoolConnections},
	}
	for _, field := range counts {
		value, err := parseCount(getenv, field.key, uint64(field.ceiling))
		if err != nil {
			return Config{}, err
		}
		*field.target = int(value)
	}
	if config.Workers >= config.PoolConnections {
		return Config{}, fmt.Errorf("PERPETUAL_WORKERS must be less than PERPETUAL_POOL_CONNECTIONS")
	}

	durations := []struct {
		key    string
		target *time.Duration
	}{
		{"PERPETUAL_LOCK_TIMEOUT", &config.LockTimeout},
		{"PERPETUAL_STATEMENT_TIMEOUT", &config.StatementTimeout},
		{"PERPETUAL_OPERATION_TIMEOUT", &config.OperationTimeout},
		{"PERPETUAL_SHUTDOWN_GRACE", &config.ShutdownGrace},
	}
	for _, field := range durations {
		value := getenv(field.key)
		if value == "" {
			continue
		}
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive duration", field.key)
		}
		*field.target = parsed
	}
	if config.LockTimeout >= config.StatementTimeout || config.StatementTimeout >= config.OperationTimeout {
		return Config{}, fmt.Errorf("timeouts must satisfy lock < statement < operation")
	}
	return config, nil
}

// parseCount returns ceiling when key is unset and otherwise accepts 1..ceiling.
func parseCount(getenv func(string) string, key string, ceiling uint64) (uint64, error) {
	value := getenv(key)
	if value == "" {
		return ceiling, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 || parsed > ceiling {
		return 0, fmt.Errorf("%s must be between 1 and %d", key, ceiling)
	}
	return parsed, nil
}

func validateDatabaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Hostname() == "" {
		return fmt.Errorf("PERPETUAL_DATABASE_URL must be a postgres URL with a host")
	}
	return nil
}

func validateListenAddress(address string) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return fmt.Errorf("PERPETUAL_LISTEN_ADDRESS must be a host:port address")
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return fmt.Errorf("PERPETUAL_LISTEN_ADDRESS port must be between 1 and 65535")
	}
	return nil
}
