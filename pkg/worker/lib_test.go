package worker

import (
	"testing"
	"time"

	"github.com/william-yangbo/kongtask/pkg/logger"
)

func TestProcessSharedOptionsCache(t *testing.T) {
	// Clear cache before test
	ClearCache()

	// Create test options
	options := &WorkerPoolOptions{
		Schema:       "test_schema",
		Concurrency:  3,
		PollInterval: 5 * time.Second,
		Logger:       logger.DefaultLogger,
	}

	// First call should compile and cache
	result1 := ProcessSharedOptions(options, nil)
	if result1 == nil {
		t.Fatal("Expected non-nil result")
	}

	// Second call should return cached result (same pointer for shared fields)
	result2 := ProcessSharedOptions(options, nil)
	if result2 == nil {
		t.Fatal("Expected non-nil result")
	}

	// Results should have same compiled values
	if result1.WorkerSchema != result2.WorkerSchema {
		t.Errorf("Expected same WorkerSchema, got %s vs %s", result1.WorkerSchema, result2.WorkerSchema)
	}

	if result1.Concurrency != result2.Concurrency {
		t.Errorf("Expected same Concurrency, got %d vs %d", result1.Concurrency, result2.Concurrency)
	}
}

func TestProcessSharedOptionsWithScope(t *testing.T) {
	// Clear cache before test
	ClearCache()

	options := &WorkerPoolOptions{
		Schema:      "test_schema",
		Concurrency: 2,
		Logger:      logger.DefaultLogger,
	}

	scope := &logger.LogScope{
		Label:    "test-worker",
		WorkerID: "worker-123",
	}

	settings := &ProcessSharedOptionsSettings{
		Scope: scope,
	}

	result := ProcessSharedOptions(options, settings)
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Verify that logger has scope applied
	if result.Logger == options.Logger {
		t.Error("Expected logger to be different when scope is applied")
	}
}

func TestGenerateCacheKey(t *testing.T) {
	options := &WorkerPoolOptions{
		Schema:              "test_schema",
		Concurrency:         3,
		PollInterval:        5 * time.Second,
		MaxPoolSize:         15,
		MaxContiguousErrors: 10,
	}

	key1 := generateCacheKey(options)
	key2 := generateCacheKey(options)

	if key1 != key2 {
		t.Errorf("Expected same cache key, got %s vs %s", key1, key2)
	}

	// Change one field
	options.Concurrency = 4
	key3 := generateCacheKey(options)

	if key1 == key3 {
		t.Error("Expected different cache key after changing concurrency")
	}
}

func TestReleasers(t *testing.T) {
	var releasers Releasers

	released := false
	releasers.Add(func() error {
		released = true
		return nil
	})

	if released {
		t.Error("Expected function not to be called yet")
	}

	err := releasers.Release()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if !released {
		t.Error("Expected function to be called")
	}
}

func TestApplyDefaultOptionsWithNewFields(t *testing.T) {
	options := &WorkerPoolOptions{}

	applyDefaultOptions(options)

	if options.Schema == "" {
		t.Error("Expected schema to be set")
	}

	if options.Concurrency == 0 {
		t.Error("Expected concurrency to be set")
	}

	if options.MaxPoolSize == 0 {
		t.Error("Expected MaxPoolSize to be set")
	}

	if options.MaxContiguousErrors == 0 {
		t.Error("Expected MaxContiguousErrors to be set")
	}

	if options.PollInterval == 0 {
		t.Error("Expected PollInterval to be set")
	}
}
