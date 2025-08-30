package migrate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/testutil"
)

// TestMigrate_InstallSchema_SecondMigrationDoesNoHarm corresponds to migrate.test.ts main test case
func TestMigrate_InstallSchema_SecondMigrationDoesNoHarm(t *testing.T) {
	dbURL, pool := testutil.StartPostgres(t)
	_ = dbURL // Keep for potential future use
	ctx := context.Background()

	migrator := NewMigrator(pool, "graphile_worker")

	// Ensure database initial state is empty
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	// Drop schema if exists
	_, err = conn.Exec(ctx, "DROP SCHEMA IF EXISTS graphile_worker CASCADE")
	require.NoError(t, err)

	// Verify graphile_worker schema doesn't exist
	var schemaExists bool
	err = conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_namespace WHERE nspname = $1)",
		"graphile_worker",
	).Scan(&schemaExists)
	require.NoError(t, err)
	assert.False(t, schemaExists, "Schema should not exist before migration")

	// Perform migration
	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	// Verify migrations table exists and has correct records
	var migrationCount int
	err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM graphile_worker.migrations").Scan(&migrationCount)
	require.NoError(t, err)
	assert.Equal(t, 1, migrationCount, "Should have exactly one migration record")

	var maxMigrationID int
	err = conn.QueryRow(ctx, "SELECT MAX(id) FROM graphile_worker.migrations").Scan(&maxMigrationID)
	require.NoError(t, err)
	assert.Equal(t, 1, maxMigrationID, "Latest migration ID should be 1")

	// Verify job functions work properly
	_, err = conn.Exec(ctx, "SELECT graphile_worker.add_job('assert_jobs_work')")
	require.NoError(t, err)

	var jobCount int
	err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM graphile_worker.jobs").Scan(&jobCount)
	require.NoError(t, err)
	assert.Equal(t, 1, jobCount, "Should have one job")

	var taskIdentifier string
	err = conn.QueryRow(ctx, "SELECT task_identifier FROM graphile_worker.jobs LIMIT 1").Scan(&taskIdentifier)
	require.NoError(t, err)
	assert.Equal(t, "assert_jobs_work", taskIdentifier)

	// Verify repeated migrations cause no issues
	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	// Ensure job count remains the same
	err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM graphile_worker.jobs").Scan(&jobCount)
	require.NoError(t, err)
	assert.Equal(t, 1, jobCount, "Job count should remain the same after repeated migrations")
}

func TestMigrate_WithExistingSchema(t *testing.T) {
	// Test behavior with existing schema
	dbURL, pool := testutil.StartPostgres(t)
	_ = dbURL
	ctx := context.Background()

	migrator := NewMigrator(pool, "graphile_worker")

	// First migration
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Second migration should have no side effects
	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	// Verify only one migration record exists
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var migrationCount int
	err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM graphile_worker.migrations").Scan(&migrationCount)
	require.NoError(t, err)
	assert.Equal(t, 1, migrationCount)
}
