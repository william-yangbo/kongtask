package worker

import (
	"testing"
	"time"
)

// TestUseNodeTimeConfiguration tests that UseNodeTime option is properly configured
func TestUseNodeTimeConfiguration(t *testing.T) {
	// Test WorkerPoolOptions with UseNodeTime
	options := &WorkerPoolOptions{
		Schema:      "test_schema",
		UseNodeTime: true,
	}

	compiled := ProcessSharedOptions(options, nil)
	if !compiled.UseNodeTime {
		t.Errorf("Expected UseNodeTime to be true, got %v", compiled.UseNodeTime)
	}

	// Test default value (should be false)
	optionsDefault := &WorkerPoolOptions{
		Schema: "test_schema",
	}

	compiledDefault := ProcessSharedOptions(optionsDefault, nil)
	if compiledDefault.UseNodeTime {
		t.Errorf("Expected UseNodeTime to default to false, got %v", compiledDefault.UseNodeTime)
	}
}

// TestUseNodeTimeWithWorkerOption tests WithUseNodeTime option function
func TestUseNodeTimeWithWorkerOption(t *testing.T) {
	worker := &Worker{
		useNodeTime: false,
	}

	// Apply option
	option := WithUseNodeTime(true)
	option(worker)

	if !worker.useNodeTime {
		t.Errorf("Expected worker.useNodeTime to be true after applying WithUseNodeTime(true), got %v", worker.useNodeTime)
	}

	// Test with false
	option = WithUseNodeTime(false)
	option(worker)

	if worker.useNodeTime {
		t.Errorf("Expected worker.useNodeTime to be false after applying WithUseNodeTime(false), got %v", worker.useNodeTime)
	}
}

// TestUseNodeTimeCacheKey tests that UseNodeTime is included in cache key generation
func TestUseNodeTimeCacheKey(t *testing.T) {
	options1 := &WorkerPoolOptions{
		Schema:      "test_schema",
		UseNodeTime: true,
	}

	options2 := &WorkerPoolOptions{
		Schema:      "test_schema",
		UseNodeTime: false,
	}

	key1 := generateCacheKey(options1)
	key2 := generateCacheKey(options2)

	if key1 == key2 {
		t.Errorf("Expected different cache keys for different UseNodeTime values, but got same key: %s", key1)
	}

	// Check that the key contains useNodeTime information
	if !contains(key1, "useNodeTime:true") {
		t.Errorf("Expected cache key to contain 'useNodeTime:true', got: %s", key1)
	}

	if !contains(key2, "useNodeTime:false") {
		t.Errorf("Expected cache key to contain 'useNodeTime:false', got: %s", key2)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr ||
		len(s) > len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestConfigurationToWorkerPoolOptions tests Configuration conversion includes UseNodeTime
func TestConfigurationToWorkerPoolOptions(t *testing.T) {
	config := &Configuration{
		Schema:       "test_schema",
		UseNodeTime:  true,
		Concurrency:  2,
		PollInterval: 5 * time.Second,
	}

	options := config.ToWorkerPoolOptions()

	if !options.UseNodeTime {
		t.Errorf("Expected UseNodeTime to be true in converted WorkerPoolOptions, got %v", options.UseNodeTime)
	}

	if options.Schema != "test_schema" {
		t.Errorf("Expected Schema to be 'test_schema', got %s", options.Schema)
	}
}
