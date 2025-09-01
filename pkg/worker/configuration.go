package worker

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ConfigurationLoader handles loading configuration from multiple sources
// Following graphile-worker's cosmiconfig pattern for configuration discovery
type ConfigurationLoader struct {
	configName  string
	configPaths []string
}

// NewConfigurationLoader creates a new configuration loader
func NewConfigurationLoader() *ConfigurationLoader {
	return &ConfigurationLoader{
		configName: "kongtask",
		configPaths: []string{
			".", // current directory
			"$HOME/.config",
			"$HOME",
		},
	}
}

// LoadConfiguration loads configuration with priority: CLI > Environment > Config File > Defaults
// This matches graphile-worker's configuration priority system
func (cl *ConfigurationLoader) LoadConfiguration() (*Configuration, error) {
	v := viper.New()

	// Set config name and paths (supports .kongtaskrc, kongtask.json, kongtask.yaml, etc.)
	v.SetConfigName(cl.configName)
	v.SetConfigType("json") // Default type, but will auto-detect

	// Add configuration paths
	for _, path := range cl.configPaths {
		v.AddConfigPath(os.ExpandEnv(path))
	}

	// Enable environment variable support with GRAPHILE_WORKER_ prefix
	// This maintains compatibility with graphile-worker
	v.SetEnvPrefix("GRAPHILE_WORKER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set defaults (matching graphile-worker defaults)
	setDefaults(v)

	// Try to read config file (not an error if it doesn't exist)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	// Parse into Configuration struct
	config := &Configuration{}
	if err := v.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return config, nil
}

// Configuration represents the complete configuration structure
// Following graphile-worker's SharedOptions pattern
type Configuration struct {
	// SharedOptions equivalent fields
	Schema string `mapstructure:"schema" json:"schema" yaml:"schema"`

	// WorkerPoolOptions equivalent fields
	Concurrency     int           `mapstructure:"concurrency" json:"concurrency" yaml:"concurrency"`
	PollInterval    time.Duration `mapstructure:"poll_interval" json:"poll_interval" yaml:"poll_interval"`
	MaxPoolSize     int           `mapstructure:"max_pool_size" json:"max_pool_size" yaml:"max_pool_size"`
	NoHandleSignals bool          `mapstructure:"no_handle_signals" json:"no_handle_signals" yaml:"no_handle_signals"`

	// Database configuration
	DatabaseURL string `mapstructure:"database_url" json:"database_url" yaml:"database_url"`

	// Advanced options
	MaxContiguousErrors int           `mapstructure:"max_contiguous_errors" json:"max_contiguous_errors" yaml:"max_contiguous_errors"`
	JobExpiry           time.Duration `mapstructure:"job_expiry" json:"job_expiry" yaml:"job_expiry"`
}

// setDefaults sets default values matching graphile-worker behavior
func setDefaults(v *viper.Viper) {
	// Schema defaults
	v.SetDefault("schema", "graphile_worker")

	// Worker pool defaults
	v.SetDefault("concurrency", 1)      // CONCURRENT_JOBS
	v.SetDefault("poll_interval", "2s") // DefaultPollInterval
	v.SetDefault("max_pool_size", 10)   // DefaultMaxPoolSize
	v.SetDefault("no_handle_signals", false)

	// Advanced defaults
	v.SetDefault("max_contiguous_errors", 10) // MaxContiguousErrors
	v.SetDefault("job_expiry", "4h")          // Default job expiry
}

// ToWorkerPoolOptions converts Configuration to WorkerPoolOptions
// This allows backward compatibility with existing APIs
func (c *Configuration) ToWorkerPoolOptions() WorkerPoolOptions {
	return WorkerPoolOptions{
		Concurrency:     c.Concurrency,
		Schema:          c.Schema,
		PollInterval:    c.PollInterval,
		NoHandleSignals: c.NoHandleSignals,
		// Logger will be set separately as it's not configurable via file
	}
}

// LoadConfigurationWithOverrides loads configuration and applies CLI overrides
// This implements the full priority chain: CLI > Environment > Config File > Defaults
func LoadConfigurationWithOverrides(overrides map[string]interface{}) (*Configuration, error) {
	loader := NewConfigurationLoader()
	config, err := loader.LoadConfiguration()
	if err != nil {
		return nil, err
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
		if databaseURL, ok := overrides["database_url"].(string); ok && databaseURL != "" {
			config.DatabaseURL = databaseURL
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
	sampleConfig := &Configuration{
		Schema:              "graphile_worker",
		Concurrency:         1,
		PollInterval:        2 * time.Second,
		MaxPoolSize:         10,
		NoHandleSignals:     false,
		DatabaseURL:         "postgres://localhost:5432/mydb",
		MaxContiguousErrors: 10,
		JobExpiry:           4 * time.Hour,
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("json")

	// Convert struct to map for viper
	configMap := map[string]interface{}{
		"schema":                sampleConfig.Schema,
		"concurrency":           sampleConfig.Concurrency,
		"poll_interval":         sampleConfig.PollInterval.String(),
		"max_pool_size":         sampleConfig.MaxPoolSize,
		"no_handle_signals":     sampleConfig.NoHandleSignals,
		"database_url":          sampleConfig.DatabaseURL,
		"max_contiguous_errors": sampleConfig.MaxContiguousErrors,
		"job_expiry":            sampleConfig.JobExpiry.String(),
	}

	for key, value := range configMap {
		v.Set(key, value)
	}

	if err := v.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write sample config: %w", err)
	}

	return nil
}
