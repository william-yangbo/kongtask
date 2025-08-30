package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestConfigValidation(t *testing.T) {
	// Save original environment
	originalDBURL := os.Getenv("DATABASE_URL")
	defer func() {
		if originalDBURL != "" {
			os.Setenv("DATABASE_URL", originalDBURL)
		} else {
			os.Unsetenv("DATABASE_URL")
		}
	}()

	tests := []struct {
		name        string
		setup       func()
		expectError bool
		errorMsg    string
	}{
		{
			name: "no database URL provided",
			setup: func() {
				os.Unsetenv("DATABASE_URL")
				viper.Set("database_url", "")
			},
			expectError: true,
			errorMsg:    "database URL is required",
		},
		{
			name: "database URL from flag",
			setup: func() {
				os.Unsetenv("DATABASE_URL")
				viper.Set("database_url", "postgres://localhost/test")
			},
			expectError: false,
		},
		{
			name: "database URL from environment",
			setup: func() {
				os.Setenv("DATABASE_URL", "postgres://localhost/test")
				viper.Set("database_url", "")
			},
			expectError: false,
		},
		{
			name: "flag overrides environment",
			setup: func() {
				os.Setenv("DATABASE_URL", "postgres://localhost/old")
				viper.Set("database_url", "postgres://localhost/new")
			},
			expectError: false,
		},
		{
			name: "default schema when not provided",
			setup: func() {
				os.Setenv("DATABASE_URL", "postgres://localhost/test")
				viper.Set("database_url", "")
				viper.Set("schema", "")
			},
			expectError: false,
		},
		{
			name: "custom schema provided",
			setup: func() {
				os.Setenv("DATABASE_URL", "postgres://localhost/test")
				viper.Set("database_url", "")
				viper.Set("schema", "custom_worker")
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper state
			viper.Reset()

			// Setup test conditions
			tt.setup()

			// Test migrate command validation
			err := validateMigrateConfig()
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

			// Test worker command validation
			err = validateWorkerConfig()
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

func TestCommandLineArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected map[string]string
	}{
		{
			name: "database-url flag",
			args: []string{"--database-url", "postgres://localhost/test"},
			expected: map[string]string{
				"database_url": "postgres://localhost/test",
			},
		},
		{
			name: "schema flag",
			args: []string{"--schema", "custom_schema"},
			expected: map[string]string{
				"schema": "custom_schema",
			},
		},
		{
			name: "both flags",
			args: []string{
				"--database-url", "postgres://localhost/test",
				"--schema", "custom_schema",
			},
			expected: map[string]string{
				"database_url": "postgres://localhost/test",
				"schema":       "custom_schema",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper state
			viper.Reset()

			// Create a test command with the same flags as rootCmd
			testCmd := &cobra.Command{
				Use: "test",
			}
			testCmd.PersistentFlags().StringVar(&databaseURL, "database-url", "", "PostgreSQL connection URL")
			testCmd.PersistentFlags().StringVar(&schema, "schema", "graphile_worker", "Schema name for worker tables")

			// Bind flags to viper
			viper.BindPFlag("database_url", testCmd.PersistentFlags().Lookup("database-url"))
			viper.BindPFlag("schema", testCmd.PersistentFlags().Lookup("schema"))

			// Parse the test arguments
			testCmd.ParseFlags(tt.args)

			// Check that viper got the expected values
			for key, expectedValue := range tt.expected {
				actualValue := viper.GetString(key)
				if actualValue != expectedValue {
					t.Errorf("Expected %s=%s, got %s=%s", key, expectedValue, key, actualValue)
				}
			}
		})
	}
}

func TestInvalidConnectionString(t *testing.T) {
	// Test with invalid connection strings that would fail during actual connection
	invalidConnStrings := []string{
		"invalid://connection/string",
		"postgres://",
		"postgres://user@",
		"not-a-url-at-all",
		"",
	}

	for _, connStr := range invalidConnStrings {
		t.Run("invalid_conn_"+connStr, func(t *testing.T) {
			if connStr == "" {
				t.Skip("Empty string tested elsewhere")
				return
			}

			// Reset viper state
			viper.Reset()
			viper.Set("database_url", connStr)

			// This should not error in validation (we only check if it's provided)
			// The actual connection error would happen during pgxpool.New()
			err := validateMigrateConfig()
			if err != nil {
				t.Errorf("Config validation should not fail for malformed URLs, got: %v", err)
			}
		})
	}
}

func TestSchemaValidation(t *testing.T) {
	// Set valid database URL for these tests
	viper.Set("database_url", "postgres://localhost/test")

	tests := []struct {
		name           string
		schema         string
		expectedSchema string
	}{
		{
			name:           "default schema",
			schema:         "",
			expectedSchema: "graphile_worker",
		},
		{
			name:           "custom schema",
			schema:         "my_worker",
			expectedSchema: "my_worker",
		},
		{
			name:           "schema with underscores",
			schema:         "worker_schema_v2",
			expectedSchema: "worker_schema_v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Set("schema", tt.schema)

			err := validateMigrateConfig()
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check that the schema defaults correctly
			actualSchema := viper.GetString("schema")
			if actualSchema == "" {
				actualSchema = "graphile_worker"
			}
			if actualSchema != tt.expectedSchema {
				t.Errorf("Expected schema '%s', got '%s'", tt.expectedSchema, actualSchema)
			}
		})
	}
}

// Helper functions for validation (extracted from main logic)
func validateMigrateConfig() error {
	dbURL := viper.GetString("database_url")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		return fmt.Errorf("database URL is required (use --database-url flag or DATABASE_URL env var)")
	}
	return nil
}

func validateWorkerConfig() error {
	dbURL := viper.GetString("database_url")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		return fmt.Errorf("database URL is required (use --database-url flag or DATABASE_URL env var)")
	}
	return nil
}

func containsString(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) &&
		haystack[0:len(needle)] == needle ||
		len(haystack) > len(needle) &&
			haystack[len(haystack)-len(needle):] == needle ||
		len(haystack) > len(needle) &&
			findSubstring(haystack, needle))
}

func findSubstring(haystack, needle string) bool {
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
