package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
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
