package worker

import "time"

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
func DefaultConfig() Config {
	return Config{
		PollInterval:        DefaultPollInterval, // 2 * time.Second (from worker.go)
		ConcurrentJobs:      ConcurrentJobs,      // 1
		MaxContiguousErrors: MaxContiguousErrors, // 10 (from worker.go)
		MaxPoolSize:         DefaultMaxPoolSize,  // 10
		Schema:              DefaultSchema,       // "graphile_worker"
	}
}
