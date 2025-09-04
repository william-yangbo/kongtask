package worker

import "time"

// TimeProvider provides time functions for testing and production use
type TimeProvider interface {
	// Now returns the current time
	Now() time.Time
}

// RealTimeProvider implements TimeProvider using the standard time package
type RealTimeProvider struct{}

// Now returns the current real time
func (rtp *RealTimeProvider) Now() time.Time {
	return time.Now()
}

// NewRealTimeProvider creates a new real time provider
func NewRealTimeProvider() TimeProvider {
	return &RealTimeProvider{}
}
