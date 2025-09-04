package integration

import (
	"time"

	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// TestTimeProvider implements worker.TimeProvider using FakeTimer
type TestTimeProvider struct {
	timer *testutil.FakeTimer
}

// NewTestTimeProvider creates a new test time provider
func NewTestTimeProvider(timer *testutil.FakeTimer) worker.TimeProvider {
	return &TestTimeProvider{timer: timer}
}

// Now returns the current fake time
func (ttp *TestTimeProvider) Now() time.Time {
	return ttp.timer.Now()
}
