// Package integration contains integration tests for kongtask
// These tests verify the complete functionality and parity with graphile-worker
package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/internal/testutil"
)

// TestMigrate_InstallSchema_SecondMigrationDoesNoHarm corresponds to migrate.test.ts main test case
// Matches TypeScript test: "migration installs schema; second migration does no harm"
func TestMigrate_InstallSchema_SecondMigrationDoesNoHarm(t *testing.T) {
	dbURL, pool := testutil.StartPostgres(t)
	_ = dbURL // Keep for potential future use
	ctx := context.Background()

	migrator := migrate.NewMigrator(pool, "graphile_worker")

	// Ensure database initial state is empty (matching TypeScript)
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	// Drop schema if exists (matching TypeScript withPgClient fresh connection logic)
	_, err = conn.Exec(ctx, "DROP SCHEMA IF EXISTS graphile_worker CASCADE")
	require.NoError(t, err)

	// We need to release and re-acquire connection after dropping schema
	// because SQL functions' plans get cached using stale OIDs (matching TypeScript comment)
	conn.Release()
	conn, err = pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	// Assert DB is empty (matching TypeScript verification)
	var graphileWorkerNamespaceBefore interface{}
	err = conn.QueryRow(ctx,
		"SELECT nspname FROM pg_catalog.pg_namespace WHERE nspname = $1",
		"graphile_worker",
	).Scan(&graphileWorkerNamespaceBefore)

	// Should get no rows error (equivalent to TypeScript falsy check)
	require.Error(t, err, "Should get no rows when schema doesn't exist")
	require.Contains(t, err.Error(), "no rows", "Should be no rows error")

	// Perform migration (matching TypeScript)
	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	// Assert migrations table exists and has relevant entries (matching TypeScript)
	var migrationCount int
	err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM graphile_worker.migrations").Scan(&migrationCount)
	require.NoError(t, err)
	assert.Equal(t, 7, migrationCount, "Should have exactly 7 migration records (including revision tracking, index optimization, and JobKeyMode)")

	// Verify first migration ID is 1 (matching TypeScript exact verification)
	var firstMigrationID int
	err = conn.QueryRow(ctx, "SELECT id FROM graphile_worker.migrations ORDER BY id ASC LIMIT 1").Scan(&firstMigrationID)
	require.NoError(t, err)
	assert.Equal(t, 1, firstMigrationID, "First migration ID should be 1 (matching TypeScript)")

	// Assert job schema files have been created (matching TypeScript add_job test)
	_, err = conn.Exec(ctx, "SELECT graphile_worker.add_job('assert_jobs_work')")
	require.NoError(t, err, "add_job function should work after migration")

	// Verify job was created correctly (matching TypeScript verification)
	var jobCount int
	err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM graphile_worker.jobs").Scan(&jobCount)
	require.NoError(t, err)
	assert.Equal(t, 1, jobCount, "Should have exactly 1 job (matching TypeScript)")

	var taskIdentifier string
	err = conn.QueryRow(ctx, "SELECT task_identifier FROM graphile_worker.jobs LIMIT 1").Scan(&taskIdentifier)
	require.NoError(t, err)
	assert.Equal(t, "assert_jobs_work", taskIdentifier, "Task identifier should match (matching TypeScript)")

	// Assert that re-migrating causes no issues (matching TypeScript exactly)
	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	// Verify job count remains the same after multiple migrations (matching TypeScript)
	err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM graphile_worker.jobs").Scan(&jobCount)
	require.NoError(t, err)
	assert.Equal(t, 1, jobCount, "Job count should remain 1 after repeated migrations (matching TypeScript)")

	var finalTaskIdentifier string
	err = conn.QueryRow(ctx, "SELECT task_identifier FROM graphile_worker.jobs LIMIT 1").Scan(&finalTaskIdentifier)
	require.NoError(t, err)
	assert.Equal(t, "assert_jobs_work", finalTaskIdentifier, "Task identifier should remain the same (matching TypeScript)")
}

// TestMigrate_WithExistingSchema tests idempotent behavior with existing schema
// (Additional test to ensure comprehensive migrate functionality coverage)
func TestMigrate_WithExistingSchema(t *testing.T) {
	dbURL, pool := testutil.StartPostgres(t)
	_ = dbURL
	ctx := context.Background()

	migrator := migrate.NewMigrator(pool, "graphile_worker")

	// First migration (should install schema)
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Verify initial state after first migration
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var initialMigrationCount int
	err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM graphile_worker.migrations").Scan(&initialMigrationCount)
	require.NoError(t, err)
	assert.Equal(t, 7, initialMigrationCount, "Should have 7 migration records after first migration")

	// Second migration should have no side effects (matching TypeScript idempotent behavior)
	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	// Third migration should also have no side effects
	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	// Verify migration count remains unchanged (idempotent behavior)
	var finalMigrationCount int
	err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM graphile_worker.migrations").Scan(&finalMigrationCount)
	require.NoError(t, err)
	assert.Equal(t, 7, finalMigrationCount, "Migration count should remain 7 after repeated migrations")

	// Verify schema functionality remains intact
	_, err = conn.Exec(ctx, "SELECT graphile_worker.add_job('idempotent_test')")
	require.NoError(t, err, "add_job function should still work after repeated migrations")

	var jobCount int
	err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM graphile_worker.jobs").Scan(&jobCount)
	require.NoError(t, err)
	assert.Equal(t, 1, jobCount, "Should have exactly 1 job after testing")
}
