package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/events"
	"github.com/william-yangbo/kongtask/pkg/runner"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// Corresponds to cron-timing.test.ts - timing-specific cron functionality tests
const (
	// January 1st, 2021, 00:00:00 UTC - matches REFERENCE_TIMESTAMP from cron-timing.test.ts
	REFERENCE_TIMESTAMP_MS = int64(1609459200000)
)

// TestCronTimingCheckTimestampCorrect corresponds to "check timestamp is correct" test
func TestCronTimingCheckTimestampCorrect(t *testing.T) {
	// Set up fake timers
	setTime := testutil.SetupFakeTimers()
	defer testutil.GetGlobalFakeTimer().Reset()

	// Set time to reference timestamp
	setTime(REFERENCE_TIMESTAMP_MS)

	// Verify the timestamp is correct (using UTC)
	expectedTime := time.Unix(REFERENCE_TIMESTAMP_MS/1000, (REFERENCE_TIMESTAMP_MS%1000)*1000000).UTC()
	currentTime := testutil.MockableTimeNow().UTC()

	t.Logf("Expected time (UTC): %v", expectedTime)
	t.Logf("Current time (UTC): %v", currentTime)

	require.Equal(t, expectedTime.Year(), currentTime.Year())
	require.Equal(t, expectedTime.Month(), currentTime.Month())
	require.Equal(t, expectedTime.Day(), currentTime.Day())
	require.Equal(t, expectedTime.Hour(), currentTime.Hour())
	require.Equal(t, expectedTime.Minute(), currentTime.Minute())
	require.Equal(t, expectedTime.Second(), currentTime.Second())

	// Should be January 1st, 2021, 00:00:00 UTC
	require.Equal(t, 2021, currentTime.Year())
	require.Equal(t, time.January, currentTime.Month())
	require.Equal(t, 1, currentTime.Day())
	require.Equal(t, 0, currentTime.Hour())
	require.Equal(t, 0, currentTime.Minute())
	require.Equal(t, 0, currentTime.Second())
}

// TestCronTimingExecutesJobWhenExpected corresponds to "executes job when expected" test
func TestCronTimingExecutesJobWhenExpected(t *testing.T) {
	dbURL, _ := testutil.StartPostgres(t)
	testutil.WithOptions(t, dbURL, TEST_SCHEMA, func(pool *pgxpool.Pool, taskHandlers map[string]worker.TaskHandler) {
		// Set up fake timers
		setTime := testutil.SetupFakeTimers()
		defer testutil.GetGlobalFakeTimer().Reset()

		// Set time to 1am
		setTime(REFERENCE_TIMESTAMP_MS + int64(testutil.HOUR/time.Millisecond))

		// Add my_task handler
		taskHandlers["my_task"] = func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			t.Logf("my_task executed with payload: %s", string(payload))
			return nil
		}

		// Set up event monitoring
		eventMonitor := testutil.NewEventMonitor(t)
		cronScheduleCalls := eventMonitor.Count(events.EventType("cron:schedule"))

		// Create test time provider
		timeProvider := testutil.NewTestTimeProvider()

		// Create and start runner with cron configuration
		runnerOpts := runner.RunnerOptions{
			TaskList:     taskHandlers,
			PgPool:       pool,
			Schema:       TEST_SCHEMA,
			Crontab:      "0 */4 * * * my_task",
			Concurrency:  1,
			PollInterval: time.Second,
			Events:       eventMonitor.GetEventBus(),
			TimeProvider: timeProvider, // Use test time provider
		}

		r, err := runner.Run(runnerOpts)
		require.NoError(t, err)
		defer func() {
			err := r.Stop()
			require.NoError(t, err)
		}()

		// Skip waiting for cron:started event for now, just give time for setup
		t.Log("Giving time for cron setup...")
		time.Sleep(2 * time.Second)

		// At 3:00:01am, no cron should have fired yet
		t.Log("Setting time to 3:00:01am...")
		setTime(REFERENCE_TIMESTAMP_MS + 3*int64(testutil.HOUR/time.Millisecond) + int64(testutil.SECOND/time.Millisecond))

		count, _ := cronScheduleCalls.Get()
		t.Logf("Cron schedule count at 3:00:01am: %d", count)
		require.Equal(t, 0, count, "No cron should have fired at 3:00:01am")

		// At 4:00:01am, cron should fire once
		t.Log("Setting time to 4:00:01am...")
		setTime(REFERENCE_TIMESTAMP_MS + 4*int64(testutil.HOUR/time.Millisecond) + int64(testutil.SECOND/time.Millisecond))

		count, lastEvent := cronScheduleCalls.Get()
		t.Logf("Cron schedule count at 4:00:01am: %d", count)
		require.Equal(t, 1, count, "Cron should have fired once at 4:00:01am")
		require.NotNil(t, lastEvent, "Should have a cron schedule event")

		// Check that jobs exist in the database
		jobs := testutil.GetJobs(t, pool, TEST_SCHEMA)
		require.Len(t, jobs, 1, "One job should be created")
		require.Equal(t, "my_task", jobs[0].TaskIdentifier)
	})
}

// TestCronTimingNoDoubleScheduleOnClockReverse corresponds to "doesn't schedule tasks twice when system clock reverses" test
func TestCronTimingNoDoubleScheduleOnClockReverse(t *testing.T) {
	dbURL, _ := testutil.StartPostgres(t)
	testutil.WithOptions(t, dbURL, TEST_SCHEMA, func(pool *pgxpool.Pool, taskHandlers map[string]worker.TaskHandler) {
		// Set up fake timers
		setTime := testutil.SetupFakeTimers()
		defer testutil.GetGlobalFakeTimer().Reset()

		// Set time to 1am
		setTime(REFERENCE_TIMESTAMP_MS + int64(testutil.HOUR/time.Millisecond))

		// Add my_task handler
		taskHandlers["my_task"] = func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			t.Logf("my_task executed with payload: %s", string(payload))
			return nil
		}

		// Set up event monitoring
		eventMonitor := testutil.NewEventMonitor(t)
		cronScheduleCalls := eventMonitor.Count(events.EventType("cron:schedule"))

		// Create test time provider
		timeProvider := testutil.NewTestTimeProvider()

		// Create and start runner with cron configuration
		runnerOpts := runner.RunnerOptions{
			TaskList:     taskHandlers,
			PgPool:       pool,
			Schema:       TEST_SCHEMA,
			Crontab:      "0 */4 * * * my_task",
			Concurrency:  1,
			PollInterval: time.Second,
			Events:       eventMonitor.GetEventBus(),
			TimeProvider: timeProvider, // Use test time provider
		}

		r, err := runner.Run(runnerOpts)
		require.NoError(t, err)
		defer func() {
			err := r.Stop()
			require.NoError(t, err)
		}()

		// Skip waiting for cron:started event for now, just give time for setup
		t.Log("Giving time for cron setup...")
		time.Sleep(2 * time.Second)

		// At 3:00:01am, no cron should have fired yet
		setTime(REFERENCE_TIMESTAMP_MS + 3*int64(testutil.HOUR/time.Millisecond) + int64(testutil.SECOND/time.Millisecond))
		time.Sleep(100 * time.Millisecond)

		count, _ := cronScheduleCalls.Get()
		require.Equal(t, 0, count)

		// At 4:00:01am, cron should fire once
		setTime(REFERENCE_TIMESTAMP_MS + 4*int64(testutil.HOUR/time.Millisecond) + int64(testutil.SECOND/time.Millisecond))
		time.Sleep(100 * time.Millisecond)

		count, _ = cronScheduleCalls.Get()
		require.Equal(t, 1, count, "Cron should have fired once")

		// REWIND TIME to 3:00:01am
		setTime(REFERENCE_TIMESTAMP_MS + 3*int64(testutil.HOUR/time.Millisecond) + int64(testutil.SECOND/time.Millisecond))
		time.Sleep(100 * time.Millisecond)

		// Advance time again to 4:00:01am
		setTime(REFERENCE_TIMESTAMP_MS + 4*int64(testutil.HOUR/time.Millisecond) + int64(testutil.SECOND/time.Millisecond))
		time.Sleep(100 * time.Millisecond)

		// Although the time was matched again, no additional tasks should have been scheduled
		count, _ = cronScheduleCalls.Get()
		require.Equal(t, 1, count, "Should still be only 1 cron execution (no double scheduling)")
	})
}

// TestCronTimingClockSkewDoesNotPreventScheduling corresponds to "clock skew doesn't prevent task from being scheduled at the right time" test
func TestCronTimingClockSkewDoesNotPreventScheduling(t *testing.T) {
	dbURL, _ := testutil.StartPostgres(t)
	testutil.WithOptions(t, dbURL, TEST_SCHEMA, func(pool *pgxpool.Pool, taskHandlers map[string]worker.TaskHandler) {
		// Set up fake timers
		setTime := testutil.SetupFakeTimers()
		defer testutil.GetGlobalFakeTimer().Reset()

		// Set time to 1am
		setTime(REFERENCE_TIMESTAMP_MS + int64(testutil.HOUR/time.Millisecond))

		// Add my_task handler
		taskHandlers["my_task"] = func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			t.Logf("my_task executed with payload: %s", string(payload))
			return nil
		}

		// Set up event monitoring
		eventMonitor := testutil.NewEventMonitor(t)
		cronScheduleCalls := eventMonitor.Count(events.EventType("cron:schedule"))

		// Create test time provider
		timeProvider := testutil.NewTestTimeProvider()

		// Create and start runner with cron configuration
		runnerOpts := runner.RunnerOptions{
			TaskList:     taskHandlers,
			PgPool:       pool,
			Schema:       TEST_SCHEMA,
			Crontab:      "0 */4 * * * my_task",
			Concurrency:  1,
			PollInterval: time.Second,
			Events:       eventMonitor.GetEventBus(),
			TimeProvider: timeProvider, // Use test time provider
		}

		r, err := runner.Run(runnerOpts)
		require.NoError(t, err)
		defer func() {
			err := r.Stop()
			require.NoError(t, err)
		}()

		// Skip waiting for cron:started event for now, just give time for setup
		t.Log("Giving time for cron setup...")
		time.Sleep(2 * time.Second)

		// At 3:00:01am, no cron should have fired yet
		setTime(REFERENCE_TIMESTAMP_MS + 3*int64(testutil.HOUR/time.Millisecond) + int64(testutil.SECOND/time.Millisecond))

		count, _ := cronScheduleCalls.Get()
		require.Equal(t, 0, count)

		// Advance time to 3:59:30am
		setTime(REFERENCE_TIMESTAMP_MS + 4*int64(testutil.HOUR/time.Millisecond) - 30*int64(testutil.SECOND/time.Millisecond))

		count, _ = cronScheduleCalls.Get()
		require.Equal(t, 0, count)

		// Jump back and forward a few times (simulating clock skew)
		for i := 0; i < 10; i++ {
			// Jump back to 3:59:00am
			setTime(REFERENCE_TIMESTAMP_MS + 4*int64(testutil.HOUR/time.Millisecond) - int64(testutil.MINUTE/time.Millisecond))
			time.Sleep(10 * time.Millisecond)

			count, _ = cronScheduleCalls.Get()
			require.Equal(t, 0, count, "Should still be 0 at iteration %d (3:59:00am)", i)

			// Jump forward to 3:59:30am
			setTime(REFERENCE_TIMESTAMP_MS + 4*int64(testutil.HOUR/time.Millisecond) - 30*int64(testutil.SECOND/time.Millisecond))
			time.Sleep(10 * time.Millisecond)

			count, _ = cronScheduleCalls.Get()
			require.Equal(t, 0, count, "Should still be 0 at iteration %d (3:59:30am)", i)
		}

		// Finally advance the clock to cron firing time (4:00:01am)
		setTime(REFERENCE_TIMESTAMP_MS + 4*int64(testutil.HOUR/time.Millisecond) + int64(testutil.SECOND/time.Millisecond))
		time.Sleep(100 * time.Millisecond)

		// Despite all the clock skew, the cron should still fire exactly once
		count, _ = cronScheduleCalls.Get()
		require.Equal(t, 1, count, "Cron should fire exactly once despite clock skew")
	})
}
