//nolint:errcheck
package env_test

import (
	"os"
	"testing"

	"github.com/william-yangbo/kongtask/pkg/env"
)

func TestGetStringWithFallback(t *testing.T) {
	// Clear any existing environment variables
	os.Unsetenv("TEST_VAR_1")
	os.Unsetenv("TEST_VAR_2")
	os.Unsetenv("TEST_VAR_3")

	// Test with no environment variables set
	result := env.GetStringWithFallback("default", "TEST_VAR_1", "TEST_VAR_2", "TEST_VAR_3")
	if result != "default" {
		t.Errorf("Expected 'default', got '%s'", result)
	}

	// Test with first variable set
	os.Setenv("TEST_VAR_1", "first")
	defer os.Unsetenv("TEST_VAR_1")

	result = env.GetStringWithFallback("default", "TEST_VAR_1", "TEST_VAR_2", "TEST_VAR_3")
	if result != "first" {
		t.Errorf("Expected 'first', got '%s'", result)
	}

	// Test with second variable set (first takes priority)
	os.Setenv("TEST_VAR_2", "second")
	defer os.Unsetenv("TEST_VAR_2")

	result = env.GetStringWithFallback("default", "TEST_VAR_1", "TEST_VAR_2", "TEST_VAR_3")
	if result != "first" {
		t.Errorf("Expected 'first' (priority), got '%s'", result)
	}

	// Test fallback when first is empty
	os.Setenv("TEST_VAR_1", "")
	result = env.GetStringWithFallback("default", "TEST_VAR_1", "TEST_VAR_2", "TEST_VAR_3")
	if result != "second" {
		t.Errorf("Expected 'second' (fallback), got '%s'", result)
	}
}

func TestGetIntWithFallback(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("TEST_INT_1")
	os.Unsetenv("TEST_INT_2")

	// Test with no environment variables
	result := env.GetIntWithFallback(42, "TEST_INT_1", "TEST_INT_2")
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}

	// Test with valid integer
	os.Setenv("TEST_INT_1", "123")
	defer os.Unsetenv("TEST_INT_1")

	result = env.GetIntWithFallback(42, "TEST_INT_1", "TEST_INT_2")
	if result != 123 {
		t.Errorf("Expected 123, got %d", result)
	}

	// Test with invalid integer (should use fallback)
	os.Setenv("TEST_INT_2", "invalid")
	defer os.Unsetenv("TEST_INT_2")

	result = env.GetIntWithFallback(42, "TEST_INT_2")
	if result != 42 {
		t.Errorf("Expected 42 (invalid int fallback), got %d", result)
	}
}

func TestGetBoolWithFallback(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("TEST_BOOL_1")

	// Test with no environment variables
	result := env.GetBoolWithFallback(false, "TEST_BOOL_1")
	if result != false {
		t.Errorf("Expected false, got %t", result)
	}

	// Test true values
	trueValues := []string{"true", "1", "yes", "on", "TRUE", "YES", "ON"}
	for _, val := range trueValues {
		os.Setenv("TEST_BOOL_1", val)
		result = env.GetBoolWithFallback(false, "TEST_BOOL_1")
		if result != true {
			t.Errorf("Expected true for '%s', got %t", val, result)
		}
	}

	// Test false values
	falseValues := []string{"false", "0", "no", "off", "FALSE", "NO", "OFF"}
	for _, val := range falseValues {
		os.Setenv("TEST_BOOL_1", val)
		result = env.GetBoolWithFallback(true, "TEST_BOOL_1")
		if result != false {
			t.Errorf("Expected false for '%s', got %t", val, result)
		}
	}

	// Test invalid boolean (should use default)
	os.Setenv("TEST_BOOL_1", "invalid")
	result = env.GetBoolWithFallback(true, "TEST_BOOL_1")
	if result != true {
		t.Errorf("Expected true (invalid bool fallback), got %t", result)
	}

	os.Unsetenv("TEST_BOOL_1")
}

func TestSchema(t *testing.T) {
	// Clear environment variable
	os.Unsetenv("GRAPHILE_WORKER_SCHEMA")

	// Test with default
	result := env.Schema("default_schema")
	if result != "default_schema" {
		t.Errorf("Expected 'default_schema', got '%s'", result)
	}

	// Test with environment variable set
	os.Setenv("GRAPHILE_WORKER_SCHEMA", "custom_schema")
	defer os.Unsetenv("GRAPHILE_WORKER_SCHEMA")

	result = env.Schema("default_schema")
	if result != "custom_schema" {
		t.Errorf("Expected 'custom_schema', got '%s'", result)
	}
}

func TestIsDebugEnabled(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("KONGTASK_DEBUG")
	os.Unsetenv("GRAPHILE_WORKER_DEBUG")

	// Test with no environment variables
	result := env.IsDebugEnabled()
	if result != false {
		t.Errorf("Expected false, got %t", result)
	}

	// Test with KONGTASK_DEBUG (higher priority)
	os.Setenv("KONGTASK_DEBUG", "true")
	defer os.Unsetenv("KONGTASK_DEBUG")

	result = env.IsDebugEnabled()
	if result != true {
		t.Errorf("Expected true with KONGTASK_DEBUG, got %t", result)
	}

	// Test priority: KONGTASK_DEBUG over GRAPHILE_WORKER_DEBUG
	os.Setenv("GRAPHILE_WORKER_DEBUG", "false")
	defer os.Unsetenv("GRAPHILE_WORKER_DEBUG")

	result = env.IsDebugEnabled()
	if result != true {
		t.Errorf("Expected true (KONGTASK_DEBUG priority), got %t", result)
	}

	// Test with only GRAPHILE_WORKER_DEBUG
	os.Unsetenv("KONGTASK_DEBUG")
	os.Setenv("GRAPHILE_WORKER_DEBUG", "1")

	result = env.IsDebugEnabled()
	if result != true {
		t.Errorf("Expected true with GRAPHILE_WORKER_DEBUG, got %t", result)
	}

	os.Unsetenv("GRAPHILE_WORKER_DEBUG")
}

func TestLogLevel(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("KONGTASK_LOG_LEVEL")
	os.Unsetenv("GRAPHILE_WORKER_LOG_LEVEL")

	// Test with no environment variables (default to "info")
	result := env.LogLevel()
	if result != "info" {
		t.Errorf("Expected 'info', got '%s'", result)
	}

	// Test with KONGTASK_LOG_LEVEL (higher priority)
	os.Setenv("KONGTASK_LOG_LEVEL", "DEBUG")
	defer os.Unsetenv("KONGTASK_LOG_LEVEL")

	result = env.LogLevel()
	if result != "debug" {
		t.Errorf("Expected 'debug', got '%s'", result)
	}

	// Test priority: KONGTASK_LOG_LEVEL over GRAPHILE_WORKER_LOG_LEVEL
	os.Setenv("GRAPHILE_WORKER_LOG_LEVEL", "ERROR")
	defer os.Unsetenv("GRAPHILE_WORKER_LOG_LEVEL")

	result = env.LogLevel()
	if result != "debug" {
		t.Errorf("Expected 'debug' (KONGTASK priority), got '%s'", result)
	}

	// Test with only GRAPHILE_WORKER_LOG_LEVEL
	os.Unsetenv("KONGTASK_LOG_LEVEL")

	result = env.LogLevel()
	if result != "error" {
		t.Errorf("Expected 'error' with GRAPHILE_WORKER_LOG_LEVEL, got '%s'", result)
	}

	os.Unsetenv("GRAPHILE_WORKER_LOG_LEVEL")
}

func TestHasPGVariables(t *testing.T) {
	// Clear environment variable
	os.Unsetenv("PGDATABASE")

	// Test with no PGDATABASE
	result := env.HasPGVariables()
	if result != false {
		t.Errorf("Expected false, got %t", result)
	}

	// Test with PGDATABASE set
	os.Setenv("PGDATABASE", "test_db")
	defer os.Unsetenv("PGDATABASE")

	result = env.HasPGVariables()
	if result != true {
		t.Errorf("Expected true with PGDATABASE, got %t", result)
	}
}

func TestConcurrency(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("KONGTASK_CONCURRENCY")
	os.Unsetenv("GRAPHILE_WORKER_CONCURRENCY")

	// Test with no environment variables
	result := env.Concurrency(4)
	if result != 4 {
		t.Errorf("Expected 4, got %d", result)
	}

	// Test with KONGTASK_CONCURRENCY (higher priority)
	os.Setenv("KONGTASK_CONCURRENCY", "8")
	defer os.Unsetenv("KONGTASK_CONCURRENCY")

	result = env.Concurrency(4)
	if result != 8 {
		t.Errorf("Expected 8 with KONGTASK_CONCURRENCY, got %d", result)
	}

	// Test priority: KONGTASK_CONCURRENCY over GRAPHILE_WORKER_CONCURRENCY
	os.Setenv("GRAPHILE_WORKER_CONCURRENCY", "12")
	defer os.Unsetenv("GRAPHILE_WORKER_CONCURRENCY")

	result = env.Concurrency(4)
	if result != 8 {
		t.Errorf("Expected 8 (KONGTASK priority), got %d", result)
	}

	// Test with only GRAPHILE_WORKER_CONCURRENCY
	os.Unsetenv("KONGTASK_CONCURRENCY")

	result = env.Concurrency(4)
	if result != 12 {
		t.Errorf("Expected 12 with GRAPHILE_WORKER_CONCURRENCY, got %d", result)
	}

	os.Unsetenv("GRAPHILE_WORKER_CONCURRENCY")
}

func TestMaxPoolSize(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("KONGTASK_MAX_POOL_SIZE")
	os.Unsetenv("GRAPHILE_WORKER_MAX_POOL_SIZE")

	// Test with no environment variables
	result := env.MaxPoolSize(10)
	if result != 10 {
		t.Errorf("Expected 10, got %d", result)
	}

	// Test with KONGTASK_MAX_POOL_SIZE (higher priority)
	os.Setenv("KONGTASK_MAX_POOL_SIZE", "20")
	defer os.Unsetenv("KONGTASK_MAX_POOL_SIZE")

	result = env.MaxPoolSize(10)
	if result != 20 {
		t.Errorf("Expected 20 with KONGTASK_MAX_POOL_SIZE, got %d", result)
	}

	// Test priority: KONGTASK_MAX_POOL_SIZE over GRAPHILE_WORKER_MAX_POOL_SIZE
	os.Setenv("GRAPHILE_WORKER_MAX_POOL_SIZE", "30")
	defer os.Unsetenv("GRAPHILE_WORKER_MAX_POOL_SIZE")

	result = env.MaxPoolSize(10)
	if result != 20 {
		t.Errorf("Expected 20 (KONGTASK priority), got %d", result)
	}

	// Test with only GRAPHILE_WORKER_MAX_POOL_SIZE
	os.Unsetenv("KONGTASK_MAX_POOL_SIZE")

	result = env.MaxPoolSize(10)
	if result != 30 {
		t.Errorf("Expected 30 with GRAPHILE_WORKER_MAX_POOL_SIZE, got %d", result)
	}

	os.Unsetenv("GRAPHILE_WORKER_MAX_POOL_SIZE")
}
