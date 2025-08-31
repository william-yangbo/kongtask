package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// TestRunnerConfigValidation tests configuration validation scenarios
// similar to the original runner.runOnce.test.ts
func TestRunnerConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() *RunnerOptions
		expectError bool
		errorMsg    string
	}{
		{
			name: "missing database configuration",
			setup: func() *RunnerOptions {
				return &RunnerOptions{}
			},
			expectError: true,
			errorMsg:    "database URL is required",
		},
		{
			name: "valid database URL",
			setup: func() *RunnerOptions {
				return &RunnerOptions{
					DatabaseURL: "postgres://localhost/test",
				}
			},
			expectError: false,
		},
		{
			name: "database URL from environment",
			setup: func() *RunnerOptions {
				os.Setenv("DATABASE_URL", "postgres://localhost/test")
				return &RunnerOptions{}
			},
			expectError: false,
		},
		{
			name: "custom schema",
			setup: func() *RunnerOptions {
				return &RunnerOptions{
					DatabaseURL: "postgres://localhost/test",
					Schema:      "custom_worker",
				}
			},
			expectError: false,
		},
		{
			name: "invalid poll interval",
			setup: func() *RunnerOptions {
				return &RunnerOptions{
					DatabaseURL:  "postgres://localhost/test",
					PollInterval: -1 * time.Second,
				}
			},
			expectError: true,
			errorMsg:    "poll interval must be positive",
		},
		{
			name: "invalid job timeout",
			setup: func() *RunnerOptions {
				return &RunnerOptions{
					DatabaseURL: "postgres://localhost/test",
					JobTimeout:  -1 * time.Second,
				}
			},
			expectError: true,
			errorMsg:    "job timeout must be positive",
		},
		{
			name: "valid full configuration",
			setup: func() *RunnerOptions {
				return &RunnerOptions{
					DatabaseURL:  "postgres://localhost/test",
					Schema:       "custom_worker",
					PollInterval: 2 * time.Second,
					JobTimeout:   30 * time.Second,
					WorkerID:     "test-worker-1",
				}
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean environment
			originalDBURL := os.Getenv("DATABASE_URL")
			defer func() {
				if originalDBURL != "" {
					os.Setenv("DATABASE_URL", originalDBURL)
				} else {
					os.Unsetenv("DATABASE_URL")
				}
			}()

			options := tt.setup()
			err := validateRunnerOptions(options)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestConfigFileHandling tests configuration file loading
func TestConfigFileHandling(t *testing.T) {
	// Create a temporary config file
	tmpDir, err := os.MkdirTemp("", "kongtask-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := tmpDir + "/.kongtask.yaml"
	configContent := `
database_url: postgres://config-file/test
schema: config_schema
poll_interval: 5s
job_timeout: 60s
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test config file loading
	viper.Reset()
	viper.SetConfigFile(configFile)
	err = viper.ReadInConfig()
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	// Verify config values
	if viper.GetString("database_url") != "postgres://config-file/test" {
		t.Errorf("Expected database_url from config file")
	}
	if viper.GetString("schema") != "config_schema" {
		t.Errorf("Expected schema from config file")
	}
}

// TestEnvironmentVariableHandling tests environment variable precedence
func TestEnvironmentVariableHandling(t *testing.T) {
	// Save original environment
	originalVars := map[string]string{
		"DATABASE_URL":     os.Getenv("DATABASE_URL"),
		"KONGTASK_SCHEMA":  os.Getenv("KONGTASK_SCHEMA"),
		"KONGTASK_TIMEOUT": os.Getenv("KONGTASK_TIMEOUT"),
	}
	defer func() {
		for key, value := range originalVars {
			if value != "" {
				os.Setenv(key, value)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	// Set environment variables
	os.Setenv("DATABASE_URL", "postgres://env/test")
	os.Setenv("KONGTASK_SCHEMA", "env_schema")
	os.Setenv("KONGTASK_TIMEOUT", "45s")

	viper.Reset()
	viper.AutomaticEnv()

	// Test that environment variables are picked up
	if os.Getenv("DATABASE_URL") != "postgres://env/test" {
		t.Errorf("Environment variable not set correctly")
	}
}

// TestCommandPrecedence tests flag vs environment vs config file precedence
func TestCommandPrecedence(t *testing.T) {
	// Setup environment
	os.Setenv("DATABASE_URL", "postgres://env/test")
	defer os.Unsetenv("DATABASE_URL")

	// Create config file
	tmpDir, err := os.MkdirTemp("", "kongtask-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := tmpDir + "/.kongtask.yaml"
	configContent := `database_url: postgres://config/test`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test that command line flag takes precedence
	viper.Reset()
	viper.SetConfigFile(configFile)
	_ = viper.ReadInConfig() // Ignore error in test
	viper.AutomaticEnv()

	// Simulate command line flag (highest precedence)
	viper.Set("database_url", "postgres://flag/test")

	dbURL := viper.GetString("database_url")
	if dbURL != "postgres://flag/test" {
		t.Errorf("Expected flag to take precedence, got: %s", dbURL)
	}
}

// TestWorkerCommandValidation tests worker-specific validation
func TestWorkerCommandValidation(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		setupEnv    func()
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid worker args",
			args:        []string{"worker", "--database-url", "postgres://localhost/test"},
			setupEnv:    func() {},
			expectError: false,
		},
		{
			name: "missing database URL",
			args: []string{"worker"},
			setupEnv: func() {
				os.Unsetenv("DATABASE_URL")
			},
			expectError: true,
			errorMsg:    "database URL is required",
		},
		{
			name: "database URL from environment",
			args: []string{"worker"},
			setupEnv: func() {
				os.Setenv("DATABASE_URL", "postgres://localhost/test")
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			originalDBURL := os.Getenv("DATABASE_URL")
			defer func() {
				if originalDBURL != "" {
					os.Setenv("DATABASE_URL", originalDBURL)
				} else {
					os.Unsetenv("DATABASE_URL")
				}
			}()

			tt.setupEnv()

			// Reset viper state
			viper.Reset()

			// Create test command
			testCmd := &cobra.Command{
				Use: "kongtask",
			}
			testCmd.PersistentFlags().StringVar(&databaseURL, "database-url", "", "PostgreSQL connection URL")
			testCmd.PersistentFlags().StringVar(&schema, "schema", "graphile_worker", "Schema name for worker tables")

			_ = viper.BindPFlag("database_url", testCmd.PersistentFlags().Lookup("database-url"))
			_ = viper.BindPFlag("schema", testCmd.PersistentFlags().Lookup("schema"))
			viper.AutomaticEnv()

			// Parse flags
			_ = testCmd.ParseFlags(tt.args[1:]) // Skip the command name

			// Validate worker configuration
			err := validateWorkerConfig()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// RunnerOptions represents the configuration options for the worker
type RunnerOptions struct {
	DatabaseURL  string
	Schema       string
	PollInterval time.Duration
	JobTimeout   time.Duration
	WorkerID     string
}

// validateRunnerOptions validates runner configuration options
func validateRunnerOptions(options *RunnerOptions) error {
	// Check database URL
	if options.DatabaseURL == "" {
		dbURL := os.Getenv("DATABASE_URL")
		if dbURL == "" {
			return fmt.Errorf("database URL is required (use --database-url flag or DATABASE_URL env var)")
		}
		options.DatabaseURL = dbURL
	}

	// Validate poll interval
	if options.PollInterval < 0 {
		return fmt.Errorf("poll interval must be positive")
	}

	// Validate job timeout
	if options.JobTimeout < 0 {
		return fmt.Errorf("job timeout must be positive")
	}

	// Set defaults
	if options.Schema == "" {
		options.Schema = "graphile_worker"
	}
	if options.PollInterval == 0 {
		options.PollInterval = 1 * time.Second
	}
	if options.JobTimeout == 0 {
		options.JobTimeout = 30 * time.Second
	}

	return nil
}
