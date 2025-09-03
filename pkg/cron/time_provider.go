package cron

import "time"

// TimeProvider is an interface for getting the current time
// This allows for dependency injection of time sources for testing
type TimeProvider interface {
	Now() time.Time
	// OnTimeChange registers a callback for when time changes (for testing)
	OnTimeChange(callback func())
}

// RealTimeProvider provides the actual system time
type RealTimeProvider struct{}

// Now returns the current system time
func (r *RealTimeProvider) Now() time.Time {
	return time.Now()
}

// OnTimeChange is a no-op for real time provider
func (r *RealTimeProvider) OnTimeChange(callback func()) {
	// No-op for real time provider since time continuously moves
}
