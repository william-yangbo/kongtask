package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// WorkerDefaults represents the default configuration structure
// This mirrors the TypeScript WorkerDefaults interface exactly
type WorkerDefaults struct {
	// How long to wait between polling for jobs.
	//
	// Note: this does NOT need to be short, because we use LISTEN/NOTIFY to be
	// notified when new jobs are added - this is just used for jobs scheduled in
	// the future, retried jobs, and in the case where LISTEN/NOTIFY fails for
	// whatever reason.
	PollInterval int `json:"pollInterval" yaml:"pollInterval"`

	// Which PostgreSQL schema should Graphile Worker use? Defaults to 'graphile_worker'.
	Schema string `json:"schema" yaml:"schema"`

	// How many errors in a row can we get fetching a job before we raise a higher
	// exception?
	MaxContiguousErrors int `json:"maxContiguousErrors" yaml:"maxContiguousErrors"`

	// Number of jobs to run concurrently
	ConcurrentJobs int `json:"concurrentJobs" yaml:"concurrentJobs"`

	// The maximum size of the PostgreSQL pool. Defaults to the node-postgres
	// default (10). Only useful when `connectionString` is given.
	MaxPoolSize int `json:"maxPoolSize" yaml:"maxPoolSize"`
}

// GetDefaults returns configuration defaults matching graphile-worker config.ts
// Sources: environmental variables, config files, and finally sensible defaults
func GetDefaults() WorkerDefaults {
	// Start with base defaults (matching TypeScript implementation)
	defaults := WorkerDefaults{
		Schema:              "graphile_worker",
		MaxContiguousErrors: 10,
		PollInterval:        2000, // milliseconds
		ConcurrentJobs:      1,
		MaxPoolSize:         10,
	}

	// Try to load from cosmiconfig-style configuration files first
	if configData := loadCosmiconfigLike(); configData != nil {
		if schema := enforceStringOrUndefined("schema", configData["schema"]); schema != "" {
			defaults.Schema = schema
		}
		if maxErrors := enforceNumberOrUndefined("maxContiguousErrors", configData["maxContiguousErrors"]); maxErrors > 0 {
			defaults.MaxContiguousErrors = maxErrors
		}
		if pollInterval := enforceNumberOrUndefined("pollInterval", configData["pollInterval"]); pollInterval > 0 {
			defaults.PollInterval = pollInterval
		}
		if concurrentJobs := enforceNumberOrUndefined("concurrentJobs", configData["concurrentJobs"]); concurrentJobs > 0 {
			defaults.ConcurrentJobs = concurrentJobs
		}
		if maxPoolSize := enforceNumberOrUndefined("maxPoolSize", configData["maxPoolSize"]); maxPoolSize > 0 {
			defaults.MaxPoolSize = maxPoolSize
		}
	}

	// Apply environmental variables last (highest priority, matching TypeScript process.env checks)
	if schema := os.Getenv("GRAPHILE_WORKER_SCHEMA"); schema != "" {
		defaults.Schema = schema
	}

	return defaults
}

// loadCosmiconfigLike searches for configuration files like cosmiconfig
// Searches for: .graphile-workerrc, .graphile-workerrc.json, .graphile-workerrc.yaml, etc.
func loadCosmiconfigLike() map[string]interface{} {
	searchPaths := []string{
		".graphile-workerrc",
		".graphile-workerrc.json",
		".graphile-workerrc.yaml",
		".graphile-workerrc.yml",
		"graphile-worker.config.json",
		"graphile-worker.config.yaml",
		"graphile-worker.config.yml",
	}

	// Start from current directory and walk up
	currentDir, err := os.Getwd()
	if err != nil {
		return nil
	}

	for {
		for _, filename := range searchPaths {
			configPath := filepath.Join(currentDir, filename)
			if data := loadConfigFile(configPath); data != nil {
				return data
			}
		}

		// Move up one directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			break // Reached root
		}
		currentDir = parentDir
	}

	return nil
}

// loadConfigFile loads and parses a configuration file
func loadConfigFile(path string) map[string]interface{} {
	// Basic path validation
	cleanPath := filepath.Clean(path)
	if cleanPath != path {
		return nil // Reject paths with directory traversal attempts
	}

	data, err := os.ReadFile(cleanPath) //#nosec G304 -- Path validated above
	if err != nil {
		return nil
	}

	var config map[string]interface{}
	ext := filepath.Ext(path)

	switch ext {
	case ".json", "":
		if err := json.Unmarshal(data, &config); err != nil {
			return nil
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil
		}
	default:
		return nil
	}

	return config
}

// enforceStringOrUndefined validates string configuration values
// This mirrors the TypeScript enforceStringOrUndefined function exactly
func enforceStringOrUndefined(keyName string, value interface{}) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	// TypeScript would throw an error here, but Go returns empty string
	return ""
}

// enforceNumberOrUndefined validates number configuration values
// This mirrors the TypeScript enforceNumberOrUndefined function exactly
func enforceNumberOrUndefined(keyName string, value interface{}) int {
	if value == nil {
		return 0
	}

	switch v := value.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		if val, err := strconv.Atoi(v); err == nil {
			return val
		}
		// TypeScript would throw an error here, but Go returns 0
		return 0
	default:
		// TypeScript would throw an error here, but Go returns 0
		return 0
	}
}

// ToConfig converts WorkerDefaults to Config structure
func (wd WorkerDefaults) ToConfig() Config {
	return Config{
		PollInterval:        time.Duration(wd.PollInterval) * time.Millisecond,
		ConcurrentJobs:      wd.ConcurrentJobs,
		MaxContiguousErrors: wd.MaxContiguousErrors,
		MaxPoolSize:         wd.MaxPoolSize,
		Schema:              wd.Schema,
	}
}

// GetDefaultsConfig returns a Config struct with all default values
// This is a convenience function that combines GetDefaults() and ToConfig()
func GetDefaultsConfig() Config {
	return GetDefaults().ToConfig()
}

// Configuration represents the complete configuration structure
// Following graphile-worker's SharedOptions pattern
type Configuration struct {
	// SharedOptions equivalent fields
	Schema      string `json:"schema" yaml:"schema"`
	UseNodeTime bool   `json:"use_node_time" yaml:"use_node_time"`

	// WorkerPoolOptions equivalent fields
	Concurrency          int           `json:"concurrency" yaml:"concurrency"`
	PollInterval         time.Duration `json:"poll_interval" yaml:"poll_interval"`
	MaxPoolSize          int           `json:"max_pool_size" yaml:"max_pool_size"`
	NoHandleSignals      bool          `json:"no_handle_signals" yaml:"no_handle_signals"`
	NoPreparedStatements bool          `json:"no_prepared_statements" yaml:"no_prepared_statements"`

	// Database configuration
	DatabaseURL string `json:"database_url" yaml:"database_url"`

	// Advanced options
	MaxContiguousErrors int           `json:"max_contiguous_errors" yaml:"max_contiguous_errors"`
	JobExpiry           time.Duration `json:"job_expiry" yaml:"job_expiry"`
}

// ToWorkerPoolOptions converts Configuration to WorkerPoolOptions
// This allows backward compatibility with existing APIs
func (c *Configuration) ToWorkerPoolOptions() WorkerPoolOptions {
	return WorkerPoolOptions{
		Concurrency:          c.Concurrency,
		Schema:               c.Schema,
		PollInterval:         c.PollInterval,
		NoHandleSignals:      c.NoHandleSignals,
		NoPreparedStatements: c.NoPreparedStatements,
		UseNodeTime:          c.UseNodeTime,
		// Logger will be set separately as it's not configurable via file
	}
}

// LoadConfigurationWithOverrides loads configuration and applies CLI overrides
// This implements the full priority chain: CLI > Environment > Config File > Defaults
// Uses the cosmiconfig-like functionality from GetDefaults()
func LoadConfigurationWithOverrides(overrides map[string]interface{}) (*Configuration, error) {
	// Get defaults using our cosmiconfig-like implementation
	defaults := GetDefaults()

	// Convert WorkerDefaults to Configuration
	config := &Configuration{
		Schema:               defaults.Schema,
		UseNodeTime:          false, // Default value (matches graphile-worker default)
		Concurrency:          defaults.ConcurrentJobs,
		PollInterval:         time.Duration(defaults.PollInterval) * time.Millisecond,
		MaxPoolSize:          defaults.MaxPoolSize,
		NoHandleSignals:      false, // Default value
		NoPreparedStatements: false, // Default value
		DatabaseURL:          "",    // Not part of defaults
		MaxContiguousErrors:  defaults.MaxContiguousErrors,
		JobExpiry:            4 * time.Hour, // Default value
	}

	// Apply CLI overrides (highest priority)
	if overrides != nil {
		if schema, ok := overrides["schema"].(string); ok && schema != "" {
			config.Schema = schema
		}
		if concurrency, ok := overrides["concurrency"].(int); ok && concurrency > 0 {
			config.Concurrency = concurrency
		}
		if pollInterval, ok := overrides["poll_interval"].(time.Duration); ok && pollInterval > 0 {
			config.PollInterval = pollInterval
		}
		if maxPoolSize, ok := overrides["max_pool_size"].(int); ok && maxPoolSize > 0 {
			config.MaxPoolSize = maxPoolSize
		}
		if noHandleSignals, ok := overrides["no_handle_signals"].(bool); ok {
			config.NoHandleSignals = noHandleSignals
		}
		if noPreparedStatements, ok := overrides["no_prepared_statements"].(bool); ok {
			config.NoPreparedStatements = noPreparedStatements
		}
		if useNodeTime, ok := overrides["use_node_time"].(bool); ok {
			config.UseNodeTime = useNodeTime
		}
		if databaseURL, ok := overrides["database_url"].(string); ok && databaseURL != "" {
			config.DatabaseURL = databaseURL
		}
		if maxContiguousErrors, ok := overrides["max_contiguous_errors"].(int); ok && maxContiguousErrors > 0 {
			config.MaxContiguousErrors = maxContiguousErrors
		}
		if jobExpiry, ok := overrides["job_expiry"].(time.Duration); ok && jobExpiry > 0 {
			config.JobExpiry = jobExpiry
		}
	}

	return config, nil
}

// ValidateConfiguration validates the configuration for correctness
func (c *Configuration) ValidateConfiguration() error {
	if c.Schema == "" {
		return fmt.Errorf("schema cannot be empty")
	}

	if c.Concurrency < 1 {
		return fmt.Errorf("concurrency must be at least 1, got %d", c.Concurrency)
	}

	if c.PollInterval < time.Millisecond {
		return fmt.Errorf("poll_interval must be at least 1ms, got %v", c.PollInterval)
	}

	if c.MaxPoolSize < 1 {
		return fmt.Errorf("max_pool_size must be at least 1, got %d", c.MaxPoolSize)
	}

	return nil
}

// SaveSampleConfiguration creates a sample configuration file
// This helps users understand available options
func SaveSampleConfiguration(path string) error {
	// Create sample config with human-readable values
	sampleConfigMap := map[string]interface{}{
		"schema":                "graphile_worker",
		"concurrency":           1,
		"poll_interval":         "2s",
		"max_pool_size":         10,
		"no_handle_signals":     false,
		"database_url":          "postgres://localhost:5432/mydb",
		"max_contiguous_errors": 10,
		"job_expiry":            "4h",
	}

	// Convert to JSON format for simplicity (no viper dependency)
	data, err := json.MarshalIndent(sampleConfigMap, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sample config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write sample config: %w", err)
	}

	return nil
}
