package testutil

import (
	"sync"
	"time"

	"github.com/william-yangbo/kongtask/pkg/cron"
)

// FakeTimer provides time mocking capabilities for tests
// Corresponds to setupFakeTimers functionality from jest-time-helpers
type FakeTimer struct {
	mu          sync.RWMutex
	currentTime time.Time
	callbacks   []func()
}

// NewFakeTimer creates a new fake timer starting at the given time
func NewFakeTimer(startTime time.Time) *FakeTimer {
	return &FakeTimer{
		currentTime: startTime,
		callbacks:   make([]func(), 0),
	}
}

// Now returns the current fake time
func (ft *FakeTimer) Now() time.Time {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	return ft.currentTime
}

// SetTime sets the fake time to a specific time.Time
func (ft *FakeTimer) SetTime(t time.Time) {
	// First, update the time
	ft.mu.Lock()
	ft.currentTime = t
	// Copy callbacks to avoid holding lock during execution
	callbacksCopy := make([]func(), len(ft.callbacks))
	copy(callbacksCopy, ft.callbacks)
	ft.mu.Unlock()

	// Execute callbacks without holding the lock
	for _, callback := range callbacksCopy {
		callback()
	}
}

// SetTimeFromTimestamp sets the fake time from a Unix timestamp (milliseconds)
// (corresponds to setTime(REFERENCE_TIMESTAMP + ...) in cron-timing.test.ts)
func (ft *FakeTimer) SetTimeFromTimestamp(timestampMs int64) {
	t := time.Unix(timestampMs/1000, (timestampMs%1000)*1000000).UTC()
	ft.SetTime(t)
}

// Reset resets the fake timer to initial state
func (ft *FakeTimer) Reset() {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.currentTime = time.Unix(1609459200, 0).UTC() // Reset to reference timestamp in UTC
	ft.callbacks = ft.callbacks[:0]                 // Clear callbacks
}

// OnTimeChange registers a callback to be called when time changes
func (ft *FakeTimer) OnTimeChange(callback func()) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.callbacks = append(ft.callbacks, callback)
}

// globalFakeTimer is the global instance used across tests
var (
	globalFakeTimer *FakeTimer
	globalMutex     sync.Mutex
)

// SetupFakeTimers initializes the fake timer system
// Returns setTime function that corresponds to cron-timing.test.ts usage
func SetupFakeTimers() func(int64) {
	globalMutex.Lock()
	defer globalMutex.Unlock()

	// Start at the reference timestamp if no time is provided
	startTime := time.Unix(1609459200, 0) // 2021-01-01 00:00:00 UTC
	globalFakeTimer = NewFakeTimer(startTime)

	return globalFakeTimer.SetTimeFromTimestamp
}

// GetGlobalFakeTimer returns the global fake timer instance
func GetGlobalFakeTimer() *FakeTimer {
	globalMutex.Lock()
	defer globalMutex.Unlock()

	if globalFakeTimer == nil {
		startTime := time.Unix(1609459200, 0).UTC() // 2021-01-01 00:00:00 UTC
		globalFakeTimer = NewFakeTimer(startTime)
	}
	return globalFakeTimer
}

// NewTestTimeProvider creates a cron.TimeProvider backed by the global fake timer
func NewTestTimeProvider() cron.TimeProvider {
	return &TestTimeProvider{timer: GetGlobalFakeTimer()}
}

// TestTimeProvider implements cron.TimeProvider using FakeTimer
type TestTimeProvider struct {
	timer *FakeTimer
}

// Now returns the current fake time
func (ttp *TestTimeProvider) Now() time.Time {
	return ttp.timer.Now()
}

// OnTimeChange registers a callback for when time changes
func (ttp *TestTimeProvider) OnTimeChange(callback func()) {
	ttp.timer.OnTimeChange(callback)
}

// MockableTimeNow returns the current time, respecting fake timers
// This should be used instead of time.Now() in code under test
func MockableTimeNow() time.Time {
	timer := GetGlobalFakeTimer()
	return timer.Now()
}

// Time constants matching helpers.ts
const (
	SECOND = time.Second
	MINUTE = time.Minute
	HOUR   = time.Hour
	DAY    = 24 * time.Hour
	WEEK   = 7 * DAY
)
