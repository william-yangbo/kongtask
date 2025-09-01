package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDefaultsWithYAMLConfigFile(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "kongtask-yaml-config-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create YAML config file
	configContent := `
schema: yaml_test_schema
maxContiguousErrors: 25
pollInterval: 5000
concurrentJobs: 4
maxPoolSize: 40
`

	configPath := filepath.Join(tempDir, ".graphile-workerrc.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to temp directory
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Test loading YAML config
	defaults := GetDefaults()
	if defaults.Schema != "yaml_test_schema" {
		t.Errorf("Expected schema 'yaml_test_schema', got '%s'", defaults.Schema)
	}
	if defaults.MaxContiguousErrors != 25 {
		t.Errorf("Expected maxContiguousErrors 25, got %d", defaults.MaxContiguousErrors)
	}
	if defaults.PollInterval != 5000 {
		t.Errorf("Expected pollInterval 5000, got %d", defaults.PollInterval)
	}
	if defaults.ConcurrentJobs != 4 {
		t.Errorf("Expected concurrentJobs 4, got %d", defaults.ConcurrentJobs)
	}
	if defaults.MaxPoolSize != 40 {
		t.Errorf("Expected maxPoolSize 40, got %d", defaults.MaxPoolSize)
	}
}

func TestConfigPriorityOrder(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "kongtask-priority-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create config file with one value
	configContent := `{
		"schema": "config_file_schema",
		"maxContiguousErrors": 15
	}`

	configPath := filepath.Join(tempDir, ".graphile-workerrc.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Set environment variable (should override config file)
	os.Setenv("GRAPHILE_WORKER_SCHEMA", "env_var_schema")
	defer os.Unsetenv("GRAPHILE_WORKER_SCHEMA")

	// Change to temp directory
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Test priority: env var should override config file
	defaults := GetDefaults()
	if defaults.Schema != "env_var_schema" {
		t.Errorf("Expected schema 'env_var_schema' (from env var), got '%s'", defaults.Schema)
	}
	// Config file value should still be used for non-env variables
	if defaults.MaxContiguousErrors != 15 {
		t.Errorf("Expected maxContiguousErrors 15 (from config file), got %d", defaults.MaxContiguousErrors)
	}
}

func TestGetDefaultsConfigIntegration(t *testing.T) {
	// Test the convenience function
	config := GetDefaultsConfig()

	// Should match defaults
	if config.Schema != "graphile_worker" {
		t.Errorf("Expected schema 'graphile_worker', got '%s'", config.Schema)
	}
	if config.MaxContiguousErrors != 10 {
		t.Errorf("Expected maxContiguousErrors 10, got %d", config.MaxContiguousErrors)
	}
	if config.ConcurrentJobs != 1 {
		t.Errorf("Expected concurrentJobs 1, got %d", config.ConcurrentJobs)
	}
	if config.MaxPoolSize != 10 {
		t.Errorf("Expected maxPoolSize 10, got %d", config.MaxPoolSize)
	}

	// PollInterval should be converted to Duration
	expectedDuration := 2000 * 1000000 // 2000ms in nanoseconds
	if config.PollInterval.Nanoseconds() != int64(expectedDuration) {
		t.Errorf("Expected pollInterval 2s, got %v", config.PollInterval)
	}
}
