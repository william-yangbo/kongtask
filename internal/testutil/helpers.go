package testutil

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// Reset resets database state (corresponds to helpers.ts reset function)
func Reset(t testing.TB, pool *pgxpool.Pool, schema string) {
	t.Helper()
	ctx := context.Background()

	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	// Drop schema
	_, err = conn.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	require.NoError(t, err)

	// Re-run migrations to restore schema
	migrator := migrate.NewMigrator(pool, schema)
	err = migrator.Migrate(ctx)
	require.NoError(t, err)
}

// WithPgPool manages pool lifecycle (corresponds to helpers.ts withPgPool)
func WithPgPool(t testing.TB, connectionString string, fn func(*pgxpool.Pool)) {
	t.Helper()
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, connectionString)
	require.NoError(t, err)
	defer pool.Close()

	fn(pool)
}

// WithPgConn manages connection lifecycle (corresponds to helpers.ts withPgClient)
func WithPgConn(t testing.TB, pool *pgxpool.Pool, fn func(*pgx.Conn)) {
	t.Helper()
	ctx := context.Background()

	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	fn(conn.Conn())
}

// WithTransaction manages transaction lifecycle (corresponds to helpers.ts withTransaction)
func WithTransaction(t testing.TB, pool *pgxpool.Pool, fn func(pgx.Tx), commitTx ...bool) {
	t.Helper()
	ctx := context.Background()

	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	require.NoError(t, err)

	defer func() {
		if len(commitTx) > 0 && commitTx[0] {
			err = tx.Commit(ctx)
			require.NoError(t, err)
		} else {
			_ = tx.Rollback(ctx) // Ignore rollback errors
		}
	}()

	fn(tx)
}

// JobCount gets the number of jobs (corresponds to helpers.ts jobCount function)
func JobCount(t testing.TB, pool *pgxpool.Pool, schema string) int {
	t.Helper()
	ctx := context.Background()

	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var count int
	err = conn.QueryRow(ctx, "SELECT COUNT(*)::int FROM "+schema+".jobs").Scan(&count)
	require.NoError(t, err)

	return count
}

// SleepUntil waits until a condition is met or timeout is reached
func SleepUntil(condition func() bool, maxDuration time.Duration) error {
	start := time.Now()
	for !condition() && time.Since(start) < maxDuration {
		time.Sleep(2 * time.Millisecond)
	}
	if !condition() {
		return &TimeoutError{Duration: time.Since(start)}
	}
	return nil
}

// TimeoutError represents a timeout waiting for a condition
type TimeoutError struct {
	Duration time.Duration
}

func (e *TimeoutError) Error() string {
	return "timeout after " + e.Duration.String()
}

// WaitForJobCount waits until the job count reaches the expected value
func WaitForJobCount(t testing.TB, pool *pgxpool.Pool, schema string, expectedCount int, timeout time.Duration) {
	t.Helper()

	err := SleepUntil(func() bool {
		return JobCount(t, pool, schema) == expectedCount
	}, timeout)
	require.NoError(t, err, "Timed out waiting for job count to reach %d", expectedCount)
}

// Job represents a job record (corresponds to helpers.ts Job interface)
type Job struct {
	ID             string          `json:"id"`
	QueueName      *string         `json:"queue_name"`
	TaskIdentifier string          `json:"task_identifier"`
	Payload        any             `json:"payload"`
	Priority       int             `json:"priority"`
	RunAt          time.Time       `json:"run_at"`
	Attempts       int             `json:"attempts"`
	MaxAttempts    int             `json:"max_attempts"`
	LastError      *string         `json:"last_error"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	LockedAt       *time.Time      `json:"locked_at"`
	LockedBy       *string         `json:"locked_by"`
	Revision       int             `json:"revision"` // New in commit 60da79a: job revision tracking
	Key            *string         `json:"key"`
	Flags          map[string]bool `json:"flags"` // New in commit fb9b249: forbidden flags support
}

// MakeMockJob creates a mock job for testing (corresponds to helpers.ts makeMockJob)
func MakeMockJob(taskIdentifier string) Job {
	createdAt := time.Now().Add(-time.Duration(rand.Intn(12345678)) * time.Millisecond)
	return Job{
		ID:             fmt.Sprintf("%d", rand.Uint32()),
		QueueName:      nil,
		TaskIdentifier: taskIdentifier,
		Payload:        map[string]any{},
		Priority:       0,
		RunAt:          time.Now().Add(-time.Duration(rand.Intn(2000)) * time.Millisecond),
		Attempts:       0,
		MaxAttempts:    25,
		LastError:      nil,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
		LockedAt:       nil,
		LockedBy:       nil,
		Revision:       0, // New in commit 60da79a: job revision tracking
		Key:            nil,
		Flags:          nil, // New in commit fb9b249: forbidden flags support
	}
}

// JobSelection holds different types of jobs for testing
type JobSelection struct {
	FailedJob    Job
	RegularJob1  Job
	LockedJob    Job
	RegularJob2  Job
	UntouchedJob Job
}

// MakeSelectionOfJobs creates jobs in various states (corresponds to helpers.ts makeSelectionOfJobs)
func MakeSelectionOfJobs(t testing.TB, pool *pgxpool.Pool, schema string, utils *worker.WorkerUtils) JobSelection {
	t.Helper()
	ctx := context.Background()

	future := time.Now().Add(60 * time.Minute)

	// Add jobs
	failedJobID, err := utils.QuickAddJob(ctx, "job1", map[string]any{"a": 1}, worker.TaskSpec{RunAt: &future})
	require.NoError(t, err)

	regularJob1ID, err := utils.QuickAddJob(ctx, "job1", map[string]any{"a": 2}, worker.TaskSpec{RunAt: &future})
	require.NoError(t, err)

	lockedJobID, err := utils.QuickAddJob(ctx, "job1", map[string]any{"a": 3}, worker.TaskSpec{RunAt: &future})
	require.NoError(t, err)

	regularJob2ID, err := utils.QuickAddJob(ctx, "job1", map[string]any{"a": 4}, worker.TaskSpec{RunAt: &future})
	require.NoError(t, err)

	untouchedJobID, err := utils.QuickAddJob(ctx, "job1", map[string]any{"a": 5}, worker.TaskSpec{RunAt: &future})
	require.NoError(t, err)

	// Modify jobs to create different states
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	// Lock one job
	_, err = conn.Exec(ctx,
		"UPDATE "+schema+".jobs SET locked_by = 'test', locked_at = now() WHERE id = $1",
		lockedJobID)
	require.NoError(t, err)

	// Fail one job permanently
	_, err = conn.Exec(ctx,
		"UPDATE "+schema+".jobs SET attempts = max_attempts, last_error = 'Failed forever' WHERE id = $1",
		failedJobID)
	require.NoError(t, err)

	return JobSelection{
		FailedJob:    Job{ID: failedJobID, TaskIdentifier: "job1"},
		RegularJob1:  Job{ID: regularJob1ID, TaskIdentifier: "job1"},
		LockedJob:    Job{ID: lockedJobID, TaskIdentifier: "job1"},
		RegularJob2:  Job{ID: regularJob2ID, TaskIdentifier: "job1"},
		UntouchedJob: Job{ID: untouchedJobID, TaskIdentifier: "job1"},
	}
}

// Sleep pauses execution for the specified duration (corresponds to helpers.ts sleep)
func Sleep(duration time.Duration) {
	time.Sleep(duration)
}

// WithEnv temporarily sets environment variables for the duration of the test
// (corresponds to helpers.ts withEnv function from commit 6edb981)
func WithEnv(t testing.TB, envOverrides map[string]string, fn func()) {
	t.Helper()

	// Save original environment values
	original := make(map[string]string)
	for key := range envOverrides {
		if val, exists := os.LookupEnv(key); exists {
			original[key] = val
		}
	}

	// Set new environment values
	for key, value := range envOverrides {
		if value == "" {
			_ = os.Unsetenv(key)
		} else {
			_ = os.Setenv(key, value)
		}
	}

	// Ensure cleanup happens
	defer func() {
		for key := range envOverrides {
			if originalVal, had := original[key]; had {
				_ = os.Setenv(key, originalVal)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}()

	fn()
}
