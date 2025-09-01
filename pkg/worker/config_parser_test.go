package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetDefaults(t *testing.T) {
	// Test basic defaults
	defaults := GetDefaults()

	if defaults.Schema != "graphile_worker" {
		t.Errorf("Expected schema 'graphile_worker', got '%s'", defaults.Schema)
	}
	if defaults.MaxContiguousErrors != 10 {
		t.Errorf("Expected maxContiguousErrors 10, got %d", defaults.MaxContiguousErrors)
	}
	if defaults.PollInterval != 2000 {
		t.Errorf("Expected pollInterval 2000, got %d", defaults.PollInterval)
	}
	if defaults.ConcurrentJobs != 1 {
		t.Errorf("Expected concurrentJobs 1, got %d", defaults.ConcurrentJobs)
	}
	if defaults.MaxPoolSize != 10 {
		t.Errorf("Expected maxPoolSize 10, got %d", defaults.MaxPoolSize)
	}
}

func TestGetDefaultsWithEnvVar(t *testing.T) {
	// Set environment variable
	if err := os.Setenv("GRAPHILE_WORKER_SCHEMA", "custom_schema"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Unsetenv("GRAPHILE_WORKER_SCHEMA"); err != nil {
			t.Errorf("Failed to unset environment variable: %v", err)
		}
	}()

	defaults := GetDefaults()
	if defaults.Schema != "custom_schema" {
		t.Errorf("Expected schema 'custom_schema', got '%s'", defaults.Schema)
	}
}

func TestGetDefaultsWithConfigFile(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "kongtask-config-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	// Create config file
	configContent := `{
		"schema": "test_schema",
		"maxContiguousErrors": 15,
		"pollInterval": 3000,
		"concurrentJobs": 2,
		"maxPoolSize": 20
	}`

	configPath := filepath.Join(tempDir, ".graphile-workerrc.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to temp directory
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Errorf("Failed to restore directory: %v", err)
		}
	}()

	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Test loading config
	defaults := GetDefaults()
	if defaults.Schema != "test_schema" {
		t.Errorf("Expected schema 'test_schema', got '%s'", defaults.Schema)
	}
	if defaults.MaxContiguousErrors != 15 {
		t.Errorf("Expected maxContiguousErrors 15, got %d", defaults.MaxContiguousErrors)
	}
	if defaults.PollInterval != 3000 {
		t.Errorf("Expected pollInterval 3000, got %d", defaults.PollInterval)
	}
	if defaults.ConcurrentJobs != 2 {
		t.Errorf("Expected concurrentJobs 2, got %d", defaults.ConcurrentJobs)
	}
	if defaults.MaxPoolSize != 20 {
		t.Errorf("Expected maxPoolSize 20, got %d", defaults.MaxPoolSize)
	}
}

func TestToConfig(t *testing.T) {
	defaults := WorkerDefaults{
		PollInterval:        2000,
		Schema:              "test_schema",
		MaxContiguousErrors: 10,
		ConcurrentJobs:      1,
		MaxPoolSize:         10,
	}

	config := defaults.ToConfig()

	if config.PollInterval != 2*time.Second {
		t.Errorf("Expected pollInterval 2s, got %v", config.PollInterval)
	}
	if config.Schema != "test_schema" {
		t.Errorf("Expected schema 'test_schema', got '%s'", config.Schema)
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
}

func TestEnforceStringOrUndefined(t *testing.T) {
	// Test valid string
	result := enforceStringOrUndefined("test", "valid_string")
	if result != "valid_string" {
		t.Errorf("Expected 'valid_string', got '%s'", result)
	}

	// Test nil
	result = enforceStringOrUndefined("test", nil)
	if result != "" {
		t.Errorf("Expected empty string, got '%s'", result)
	}

	// Test invalid type
	result = enforceStringOrUndefined("test", 123)
	if result != "" {
		t.Errorf("Expected empty string, got '%s'", result)
	}
}

func TestEnforceNumberOrUndefined(t *testing.T) {
	// Test valid int
	result := enforceNumberOrUndefined("test", 42)
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}

	// Test valid float
	result = enforceNumberOrUndefined("test", 42.0)
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}

	// Test valid string number
	result = enforceNumberOrUndefined("test", "42")
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}

	// Test nil
	result = enforceNumberOrUndefined("test", nil)
	if result != 0 {
		t.Errorf("Expected 0, got %d", result)
	}

	// Test invalid string
	result = enforceNumberOrUndefined("test", "invalid")
	if result != 0 {
		t.Errorf("Expected 0, got %d", result)
	}

	// Test invalid type
	result = enforceNumberOrUndefined("test", []string{"invalid"})
	if result != 0 {
		t.Errorf("Expected 0, got %d", result)
	}
}
