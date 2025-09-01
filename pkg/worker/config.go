package worker

import (
	"os"
	"strconv"
	"time"
)

// Configuration constants matching graphile-worker config.ts
// These ensure 100% compatibility with the TypeScript implementation

// CONCURRENT_JOBS equivalent (1)
// Number of jobs to run concurrently
const ConcurrentJobs = 1

// Additional configuration constants for KongTask
const (
	// Default max pool size (commonly used in CLI and configurations)
	DefaultMaxPoolSize = 10

	// Default schema name
	DefaultSchema = "graphile_worker"
)

// Type-safe configuration helper
type Config struct {
	PollInterval        time.Duration
	ConcurrentJobs      int
	MaxContiguousErrors int
	MaxPoolSize         int
	Schema              string
}

// DefaultConfig returns configuration with all default values
// matching graphile-worker config.ts constants
// Now supports loading from configuration file via LoadConfigurationWithOverrides
func DefaultConfig() Config {
	return Config{
		PollInterval:        DefaultPollInterval, // 2 * time.Second (from worker.go)
		ConcurrentJobs:      ConcurrentJobs,      // 1
		MaxContiguousErrors: MaxContiguousErrors, // 10 (from worker.go)
		MaxPoolSize:         DefaultMaxPoolSize,  // 10
		Schema:              getSchemaFromEnv(),  // "graphile_worker" or from env
	}
}

// LoadConfigWithPriority loads configuration with full priority support
// Priority: CLI overrides > Environment variables > Config file > Defaults
func LoadConfigWithPriority(cliOverrides map[string]interface{}) (*Configuration, error) {
	return LoadConfigurationWithOverrides(cliOverrides)
}

// getSchemaFromEnv returns the schema name from environment variable or default
// This aligns with graphile-worker commit 5e455c0 behavior
func getSchemaFromEnv() string {
	if schema := os.Getenv("GRAPHILE_WORKER_SCHEMA"); schema != "" {
		return schema
	}
	return DefaultSchema
}

// applyDefaultOptions applies default values to WorkerPoolOptions
// This ensures consistency with graphile-worker behavior
func applyDefaultOptions(options *WorkerPoolOptions) {
	if options.Schema == "" {
		options.Schema = getSchemaFromEnv()
	}
	if options.PollInterval == 0 {
		options.PollInterval = DefaultPollInterval
	}
	if options.Concurrency == 0 {
		options.Concurrency = ConcurrentJobs
	}
}

// getEnvInt returns an integer from environment variable or default value
func getEnvInt(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getEnvDuration returns a duration from environment variable or default value
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			return duration
		}
	}
	return defaultValue
}
