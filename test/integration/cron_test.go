package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/events"
	"github.com/william-yangbo/kongtask/pkg/runner"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// Corresponds to cron.test.ts - comprehensive cron functionality tests
const (
	TEST_SCHEMA   = "kongtask"
	CRONTAB_DO_IT = "0 */4 * * * do_it ?fill=1d"
	FOUR_HOURS    = 4 * time.Hour
)

// TestCronRegistersIdentifiers corresponds to "registers identifiers" test in cron.test.ts
func TestCronRegistersIdentifiers(t *testing.T) {
	dbURL, _ := testutil.StartPostgres(t)
	testutil.WithOptions(t, dbURL, TEST_SCHEMA, func(pool *pgxpool.Pool, taskHandlers map[string]worker.TaskHandler) {
		// Add do_it task handler
		taskHandlers["do_it"] = func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			t.Logf("do_it task executed with payload: %s", string(payload))
			return nil
		}

		// Initially, no known crontabs should exist
		{
			known := testutil.GetKnown(t, pool, TEST_SCHEMA)
			require.Len(t, known, 0)
		}

		// Create and start runner with cron configuration
		runnerOpts := runner.RunnerOptions{
			TaskList:     taskHandlers,
			PgPool:       pool, // Use existing pool instead of creating new one
			Schema:       TEST_SCHEMA,
			Crontab:      CRONTAB_DO_IT,
			Concurrency:  1,
			PollInterval: time.Second,
		}

		r, err := runner.Run(runnerOpts)
		require.NoError(t, err)

		// Let it run briefly to register the cron items
		time.Sleep(2 * time.Second)

		// Stop the runner
		err = r.Stop()
		require.NoError(t, err)

		// Verify the identifier was registered
		{
			known := testutil.GetKnown(t, pool, TEST_SCHEMA)
			require.Len(t, known, 1)
			require.Equal(t, "do_it", known[0].Identifier)
			require.False(t, known[0].KnownSince.IsZero())
			require.Nil(t, known[0].LastExecution) // Should be nil on first registration

			// No jobs should be scheduled yet
			jobs := testutil.GetJobs(t, pool, TEST_SCHEMA)
			require.Len(t, jobs, 0)
		}
	})
}

// TestCronBackfillsIfIdentifierAlreadyRegistered5h corresponds to "backfills if identifier already registered (5h)" test
func TestCronBackfillsIfIdentifierAlreadyRegistered5h(t *testing.T) {
	dbURL, _ := testutil.StartPostgres(t)
	testutil.WithOptions(t, dbURL, TEST_SCHEMA, func(pool *pgxpool.Pool, taskHandlers map[string]worker.TaskHandler) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Add do_it task handler
		taskHandlers["do_it"] = func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			t.Logf("do_it task executed with payload: %s", string(payload))
			return nil
		}

		// Calculate expected time (current time rounded to 4-hour boundary)
		now := time.Now()
		expectedTime := now.Truncate(FOUR_HOURS)

		// Pre-register the identifier with last execution 5 hours ago
		testutil.WithPgConn(t, pool, func(conn *pgx.Conn) {
			query := fmt.Sprintf(`
				INSERT INTO %s.known_crontabs (
					identifier,
					known_since,
					last_execution
				)
				VALUES (
					'do_it',
					NOW() - interval '14 days',
					NOW() - interval '5 hours'
				)
			`, TEST_SCHEMA)

			_, err := conn.Exec(ctx, query)
			require.NoError(t, err)
		})

		// Set up event monitoring
		eventMonitor := testutil.NewEventMonitor(t)

		// Create and start runner with cron configuration and event monitoring
		runnerOpts := runner.RunnerOptions{
			TaskList:     taskHandlers,
			PgPool:       pool, // Use existing pool instead of creating new one
			Schema:       TEST_SCHEMA,
			Crontab:      CRONTAB_DO_IT,
			Concurrency:  1,
			PollInterval: time.Second,
			Events:       eventMonitor.GetEventBus(),
		}

		r, err := runner.Run(runnerOpts)
		require.NoError(t, err)

		// Wait for cron to finish backfilling
		eventMonitor.WaitForEventCount(t, events.EventType("cron:started"), 1, 10*time.Second)

		// Stop the runner
		err = r.Stop()
		require.NoError(t, err)

		// Verify the backfill results
		{
			known := testutil.GetKnown(t, pool, TEST_SCHEMA)
			require.Len(t, known, 1)
			require.Equal(t, "do_it", known[0].Identifier)
			require.False(t, known[0].KnownSince.IsZero())
			require.NotNil(t, known[0].LastExecution)

			// Check that last execution time is reasonable (should be recent)
			lx := *known[0].LastExecution
			timeDiff := lx.Sub(expectedTime).Abs()
			require.True(t,
				timeDiff < FOUR_HOURS,
				"Last execution time %v should be within 4 hours of expected time %v",
				lx, expectedTime)

			// For a 5-hour window with a job that runs every 4 hours, there should be 1 or 2 jobs
			jobs := testutil.GetJobs(t, pool, TEST_SCHEMA)
			require.GreaterOrEqual(t, len(jobs), 1)
			require.LessOrEqual(t, len(jobs), 2)

			if len(jobs) > 0 {
				require.Equal(t, "do_it", jobs[0].TaskIdentifier)
			}
		}
	})
}

// TestCronBackfillsIfIdentifierAlreadyRegistered25h corresponds to "backfills if identifier already registered (25h)" test
func TestCronBackfillsIfIdentifierAlreadyRegistered25h(t *testing.T) {
	dbURL, _ := testutil.StartPostgres(t)
	testutil.WithOptions(t, dbURL, TEST_SCHEMA, func(pool *pgxpool.Pool, taskHandlers map[string]worker.TaskHandler) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Add do_it task handler
		taskHandlers["do_it"] = func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			t.Logf("do_it task executed with payload: %s", string(payload))
			return nil
		}

		// Calculate expected time (current time rounded to 4-hour boundary)
		now := time.Now()
		expectedTime := now.Truncate(FOUR_HOURS)

		// Pre-register the identifier with last execution 25 hours ago
		testutil.WithPgConn(t, pool, func(conn *pgx.Conn) {
			query := fmt.Sprintf(`
				INSERT INTO %s.known_crontabs (
					identifier,
					known_since,
					last_execution
				)
				VALUES (
					'do_it',
					NOW() - interval '14 days',
					NOW() - interval '25 hours'
				)
			`, TEST_SCHEMA)

			_, err := conn.Exec(ctx, query)
			require.NoError(t, err)
		})

		// Set up event monitoring
		eventMonitor := testutil.NewEventMonitor(t)

		// Pre-subscribe to the events we're interested in
		cronStartedCounter := eventMonitor.Count(events.EventType("cron:started"))

		// Create and start runner with cron configuration and event monitoring
		runnerOpts := runner.RunnerOptions{
			TaskList:     taskHandlers,
			PgPool:       pool, // Use existing pool instead of creating new one
			Schema:       TEST_SCHEMA,
			Crontab:      CRONTAB_DO_IT,
			Concurrency:  1,
			PollInterval: time.Second,
			Events:       eventMonitor.GetEventBus(),
		}

		r, err := runner.Run(runnerOpts)
		require.NoError(t, err)

		// Wait for cron to finish backfilling
		err = testutil.SleepUntil(func() bool {
			count, _ := cronStartedCounter.Get()
			return count >= 1
		}, 15*time.Second)
		require.NoError(t, err, "Timed out waiting for cron:started event")

		// Stop the runner
		err = r.Stop()
		require.NoError(t, err)

		// Verify the backfill results
		{
			known := testutil.GetKnown(t, pool, TEST_SCHEMA)
			require.Len(t, known, 1)
			require.Equal(t, "do_it", known[0].Identifier)
			require.False(t, known[0].KnownSince.IsZero())
			require.NotNil(t, known[0].LastExecution)

			// Check that last execution time is reasonable
			lx := *known[0].LastExecution
			timeDiff := lx.Sub(expectedTime).Abs()
			require.True(t,
				timeDiff < FOUR_HOURS,
				"Last execution time %v should be within 4 hours of expected time %v",
				lx, expectedTime)

			// For a 25-hour window with a job that runs every 4 hours, there should be 6 or 7 jobs
			jobs := testutil.GetJobs(t, pool, TEST_SCHEMA)
			require.GreaterOrEqual(t, len(jobs), 6)
			require.LessOrEqual(t, len(jobs), 7)

			// All jobs should be for the do_it task
			for _, job := range jobs {
				require.Equal(t, "do_it", job.TaskIdentifier)
			}
		}
	})
}
