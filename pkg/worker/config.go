package worker

import (
	"time"

	"github.com/william-yangbo/kongtask/pkg/env"
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

// Config represents the legacy configuration structure
// NOTE: This is kept for backward compatibility with existing code
// New code should use Configuration struct from config_parser.go
type Config struct {
	PollInterval        time.Duration
	ConcurrentJobs      int
	MaxContiguousErrors int
	MaxPoolSize         int
	Schema              string
}

// DefaultConfig returns configuration with all default values
// matching graphile-worker config.ts constants
// Delegates to the new cosmiconfig-like implementation
func DefaultConfig() Config {
	return GetDefaultsConfig()
}

// getSchemaFromEnv returns the schema name from environment variable or default
// This aligns with graphile-worker commit 5e455c0 behavior
func getSchemaFromEnv() string {
	return env.Schema(DefaultSchema)
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
	if options.MaxPoolSize == 0 {
		options.MaxPoolSize = DefaultMaxPoolSize
	}
	if options.MaxContiguousErrors == 0 {
		options.MaxContiguousErrors = MaxContiguousErrors
	}
}
