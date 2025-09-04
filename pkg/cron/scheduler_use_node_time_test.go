package cron

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/events"
	"github.com/william-yangbo/kongtask/pkg/logger"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// TestSchedulerUseNodeTime tests that UseNodeTime option is properly configured and used
func TestSchedulerUseNodeTime(t *testing.T) {
	// Mock dependencies
	mockPgPool := &pgxpool.Pool{} // This would be properly initialized in real tests
	mockWorkerUtils := &worker.WorkerUtils{}
	mockEventBus := &events.EventBus{}
	mockLogger := logger.DefaultLogger

	// Test with UseNodeTime = true
	configWithNodeTime := SchedulerConfig{
		PgPool:      mockPgPool,
		Schema:      "test_schema",
		WorkerUtils: mockWorkerUtils,
		Events:      mockEventBus,
		Logger:      mockLogger,
		UseNodeTime: true,
	}

	schedulerWithNodeTime := NewScheduler(configWithNodeTime)
	defaultScheduler := schedulerWithNodeTime.(*DefaultScheduler)

	if !defaultScheduler.useNodeTime {
		t.Errorf("Expected useNodeTime to be true, got %v", defaultScheduler.useNodeTime)
	}

	// Test with UseNodeTime = false (default)
	configWithoutNodeTime := SchedulerConfig{
		PgPool:      mockPgPool,
		Schema:      "test_schema",
		WorkerUtils: mockWorkerUtils,
		Events:      mockEventBus,
		Logger:      mockLogger,
		UseNodeTime: false,
	}

	schedulerWithoutNodeTime := NewScheduler(configWithoutNodeTime)
	defaultSchedulerWithoutNodeTime := schedulerWithoutNodeTime.(*DefaultScheduler)

	if defaultSchedulerWithoutNodeTime.useNodeTime {
		t.Errorf("Expected useNodeTime to be false, got %v", defaultSchedulerWithoutNodeTime.useNodeTime)
	}
}

// TestSchedulerUseNodeTimeDefault tests that UseNodeTime defaults to false
func TestSchedulerUseNodeTimeDefault(t *testing.T) {
	// Mock dependencies
	mockPgPool := &pgxpool.Pool{}
	mockWorkerUtils := &worker.WorkerUtils{}
	mockEventBus := &events.EventBus{}
	mockLogger := logger.DefaultLogger

	// Test default behavior (UseNodeTime not explicitly set)
	config := SchedulerConfig{
		PgPool:      mockPgPool,
		Schema:      "test_schema",
		WorkerUtils: mockWorkerUtils,
		Events:      mockEventBus,
		Logger:      mockLogger,
		// UseNodeTime not set, should default to false
	}

	scheduler := NewScheduler(config)
	defaultScheduler := scheduler.(*DefaultScheduler)

	if defaultScheduler.useNodeTime {
		t.Errorf("Expected useNodeTime to default to false, got %v", defaultScheduler.useNodeTime)
	}
}

// TestSchedulerTimeProviderConsistency tests that the time provider uses Go time
// which is equivalent to Node.js time when useNodeTime is true
func TestSchedulerTimeProviderConsistency(t *testing.T) {
	// Mock dependencies
	mockPgPool := &pgxpool.Pool{}
	mockWorkerUtils := &worker.WorkerUtils{}
	mockEventBus := &events.EventBus{}
	mockLogger := logger.DefaultLogger

	config := SchedulerConfig{
		PgPool:      mockPgPool,
		Schema:      "test_schema",
		WorkerUtils: mockWorkerUtils,
		Events:      mockEventBus,
		Logger:      mockLogger,
		UseNodeTime: true,
		// TimeProvider will default to RealTimeProvider
	}

	scheduler := NewScheduler(config)
	defaultScheduler := scheduler.(*DefaultScheduler)

	// Verify that the time provider returns Go system time
	before := time.Now()
	providerTime := defaultScheduler.timeProvider.Now()
	after := time.Now()

	// The provider time should be between before and after (allowing for small execution time)
	if providerTime.Before(before) || providerTime.After(after.Add(100*time.Millisecond)) {
		t.Errorf("TimeProvider.Now() returned time outside expected range. Before: %v, Provider: %v, After: %v",
			before, providerTime, after)
	}
}

// MockTimeProvider for testing time-dependent behavior
type MockTimeProvider struct {
	currentTime time.Time
	callback    func()
}

func (m *MockTimeProvider) Now() time.Time {
	return m.currentTime
}

func (m *MockTimeProvider) OnTimeChange(callback func()) {
	m.callback = callback
}

func (m *MockTimeProvider) SetTime(t time.Time) {
	m.currentTime = t
	if m.callback != nil {
		m.callback()
	}
}

// TestSchedulerWithMockTimeProvider tests scheduler behavior with controlled time
func TestSchedulerWithMockTimeProvider(t *testing.T) {
	// Mock dependencies
	mockPgPool := &pgxpool.Pool{}
	mockWorkerUtils := &worker.WorkerUtils{}
	mockEventBus := &events.EventBus{}
	mockLogger := logger.DefaultLogger

	// Create mock time provider
	mockTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	mockTimeProvider := &MockTimeProvider{currentTime: mockTime}

	config := SchedulerConfig{
		PgPool:       mockPgPool,
		Schema:       "test_schema",
		WorkerUtils:  mockWorkerUtils,
		Events:       mockEventBus,
		Logger:       mockLogger,
		UseNodeTime:  true,
		TimeProvider: mockTimeProvider,
	}

	scheduler := NewScheduler(config)
	defaultScheduler := scheduler.(*DefaultScheduler)

	// Verify that the scheduler uses the mock time provider
	schedulerTime := defaultScheduler.timeProvider.Now()
	if !schedulerTime.Equal(mockTime) {
		t.Errorf("Expected scheduler to use mock time %v, got %v", mockTime, schedulerTime)
	}

	// Verify useNodeTime is set correctly
	if !defaultScheduler.useNodeTime {
		t.Errorf("Expected useNodeTime to be true, got %v", defaultScheduler.useNodeTime)
	}
}
