// Package integration contains integration tests for kongtask
// These tests verify the complete functionality and parity with graphile-worker
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// runOnceErrorAssertion helper function to test error cases
// (parity with graphile-worker runner.runOnce.test.ts)
func runOnceErrorAssertion(t *testing.T, options worker.RunnerOptions, expectedMessage string) {
	ctx := context.Background()
	err := worker.RunOnce(ctx, options)
	require.Error(t, err)
	require.Contains(t, err.Error(), expectedMessage)
}

// TestRunOnceValidationParity tests validation error conditions
// (parity with graphile-worker runner.runOnce.test.ts validation tests)
func TestRunOnceValidationParity(t *testing.T) {
	t.Run("requires task list or directory", func(t *testing.T) {
		options := worker.RunnerOptions{}
		runOnceErrorAssertion(t, options, "you must specify either `options.taskList` or `options.taskDirectory")
	})

	t.Run("task list and directory are mutually exclusive", func(t *testing.T) {
		options := worker.RunnerOptions{
			TaskDirectory: "foo",
			TaskList:      map[string]worker.TaskHandler{"task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error { return nil }},
		}
		runOnceErrorAssertion(t, options, "exactly one of either `taskDirectory` or `taskList` should be set")
	})

	t.Run("requires connection info", func(t *testing.T) {
		options := worker.RunnerOptions{
			TaskList: map[string]worker.TaskHandler{"task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error { return nil }},
		}
		runOnceErrorAssertion(t, options, "you must either specify `pgPool` or `connectionString`, or you must make the `DATABASE_URL` or `PG*` environmental variables available")
	})

	t.Run("connection string and pgPool are mutually exclusive", func(t *testing.T) {
		dbURL, pool := testutil.StartPostgres(t)

		options := worker.RunnerOptions{
			TaskList:         map[string]worker.TaskHandler{"task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error { return nil }},
			ConnectionString: dbURL,
			PgPool:           pool,
		}
		runOnceErrorAssertion(t, options, "both `pgPool` and `connectionString` are set, at most one of these options should be provided")
	})
}

// TestRunOnceConnectionParity tests various connection methods
// (parity with graphile-worker runner.runOnce.test.ts connection tests)
func TestRunOnceConnectionParity(t *testing.T) {
	t.Run("works with DATABASE_URL environment variable", func(t *testing.T) {
		dbURL, pool := testutil.StartPostgres(t)
		ctx := context.Background()

		// Reset and migrate database
		testutil.Reset(t, pool, "graphile_worker")
		migrator := migrate.NewMigrator(pool, "graphile_worker")
		err := migrator.Migrate(ctx)
		require.NoError(t, err)

		// Set DATABASE_URL environment variable
		originalURL := os.Getenv("DATABASE_URL")
		_ = os.Setenv("DATABASE_URL", dbURL)
		defer func() {
			if originalURL == "" {
				_ = os.Unsetenv("DATABASE_URL")
			} else {
				_ = os.Setenv("DATABASE_URL", originalURL)
			}
		}()

		options := worker.RunnerOptions{
			TaskList: map[string]worker.TaskHandler{"task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error { return nil }},
		}

		err = worker.RunOnce(ctx, options)
		require.NoError(t, err, "Should work with just DATABASE_URL")
	})

	t.Run("works with connection string", func(t *testing.T) {
		dbURL, pool := testutil.StartPostgres(t)
		ctx := context.Background()

		// Reset and migrate database
		testutil.Reset(t, pool, "graphile_worker")
		migrator := migrate.NewMigrator(pool, "graphile_worker")
		err := migrator.Migrate(ctx)
		require.NoError(t, err)

		options := worker.RunnerOptions{
			TaskList:         map[string]worker.TaskHandler{"task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error { return nil }},
			ConnectionString: dbURL,
		}

		err = worker.RunOnce(ctx, options)
		require.NoError(t, err, "Should work with just connectionString")
	})

	t.Run("works with pgPool", func(t *testing.T) {
		_, pool := testutil.StartPostgres(t)
		ctx := context.Background()

		// Reset and migrate database
		testutil.Reset(t, pool, "graphile_worker")
		migrator := migrate.NewMigrator(pool, "graphile_worker")
		err := migrator.Migrate(ctx)
		require.NoError(t, err)

		options := worker.RunnerOptions{
			TaskList: map[string]worker.TaskHandler{"task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error { return nil }},
			PgPool:   pool,
		}

		err = worker.RunOnce(ctx, options)
		require.NoError(t, err, "Should work with just pgPool")
	})

	t.Run("works with PG* environment variables", func(t *testing.T) {
		_, pool := testutil.StartPostgres(t)
		ctx := context.Background()

		// Reset and migrate database
		testutil.Reset(t, pool, "graphile_worker")
		migrator := migrate.NewMigrator(pool, "graphile_worker")
		err := migrator.Migrate(ctx)
		require.NoError(t, err)

		// Extract connection parameters from pool
		config := pool.Config()
		host := config.ConnConfig.Host
		port := config.ConnConfig.Port
		database := config.ConnConfig.Database
		user := config.ConnConfig.User
		password := config.ConnConfig.Password

		// Set PG* environment variables
		originalEnvs := map[string]string{
			"PGHOST":       os.Getenv("PGHOST"),
			"PGPORT":       os.Getenv("PGPORT"),
			"PGDATABASE":   os.Getenv("PGDATABASE"),
			"PGUSER":       os.Getenv("PGUSER"),
			"PGPASSWORD":   os.Getenv("PGPASSWORD"),
			"DATABASE_URL": os.Getenv("DATABASE_URL"),
		}
		defer func() {
			for key, val := range originalEnvs {
				if val == "" {
					_ = os.Unsetenv(key)
				} else {
					_ = os.Setenv(key, val)
				}
			}
		}()

		_ = os.Setenv("PGHOST", host)
		_ = os.Setenv("PGPORT", fmt.Sprintf("%d", port))
		_ = os.Setenv("PGDATABASE", database)
		_ = os.Setenv("PGUSER", user)
		_ = os.Setenv("PGPASSWORD", password)
		_ = os.Unsetenv("DATABASE_URL") // Remove DATABASE_URL to force PG* usage

		options := worker.RunnerOptions{
			TaskList: map[string]worker.TaskHandler{"task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error { return nil }},
		}

		err = worker.RunOnce(ctx, options)
		require.NoError(t, err, "Should work with PG* environment variables")
	})
}

// TestRunOnceJobProcessingParity tests actual job processing functionality
// (parity with graphile-worker runner.runOnce.test.ts processing tests)
func TestRunOnceJobProcessingParity(t *testing.T) {
	t.Run("processes multiple jobs correctly", func(t *testing.T) {
		_, pool := testutil.StartPostgres(t)
		ctx := context.Background()

		// Reset and migrate database
		testutil.Reset(t, pool, "graphile_worker")
		migrator := migrate.NewMigrator(pool, "graphile_worker")
		err := migrator.Migrate(ctx)
		require.NoError(t, err)

		// Add some jobs
		utils := worker.NewWorkerUtils(pool, "graphile_worker")
		_, err = utils.QuickAddJob(ctx, "test_task", map[string]interface{}{"message": "hello"})
		require.NoError(t, err)
		_, err = utils.QuickAddJob(ctx, "test_task", map[string]interface{}{"message": "world"})
		require.NoError(t, err)

		// Verify jobs exist
		jobCount := testutil.JobCount(t, pool, "graphile_worker")
		require.Equal(t, 2, jobCount, "Should have 2 jobs before processing")

		// Track processed jobs
		var processedMessages []string

		options := worker.RunnerOptions{
			TaskList: map[string]worker.TaskHandler{
				"test_task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
					var data map[string]interface{}
					err := json.Unmarshal(payload, &data)
					if err == nil {
						processedMessages = append(processedMessages, data["message"].(string))
					}
					return err
				},
			},
			PgPool: pool,
		}

		err = worker.RunOnce(ctx, options)
		require.NoError(t, err, "RunOnce should complete successfully")

		// Verify all jobs were processed
		require.Len(t, processedMessages, 2, "Should have processed 2 jobs")
		require.Contains(t, processedMessages, "hello", "Should have processed 'hello' message")
		require.Contains(t, processedMessages, "world", "Should have processed 'world' message")

		// Verify no jobs remain
		finalJobCount := testutil.JobCount(t, pool, "graphile_worker")
		require.Equal(t, 0, finalJobCount, "Should have no jobs after processing")
	})

	t.Run("handles no jobs gracefully", func(t *testing.T) {
		_, pool := testutil.StartPostgres(t)
		ctx := context.Background()

		// Reset and migrate database
		testutil.Reset(t, pool, "graphile_worker")
		migrator := migrate.NewMigrator(pool, "graphile_worker")
		err := migrator.Migrate(ctx)
		require.NoError(t, err)

		// Verify no jobs exist
		jobCount := testutil.JobCount(t, pool, "graphile_worker")
		require.Equal(t, 0, jobCount, "Should have no jobs initially")

		options := worker.RunnerOptions{
			TaskList: map[string]worker.TaskHandler{
				"test_task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
					t.Error("This task should not be called when no jobs are available")
					return nil
				},
			},
			PgPool: pool,
		}

		err = worker.RunOnce(ctx, options)
		require.NoError(t, err, "RunOnce should complete successfully even with no jobs")
	})
}

// TestRunOnceCustomSchemaParity tests custom schema functionality
// (parity with graphile-worker custom schema support)
func TestRunOnceCustomSchemaParity(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx := context.Background()

	customSchema := "custom_worker"

	// Reset and migrate database with custom schema
	testutil.Reset(t, pool, customSchema)
	migrator := migrate.NewMigrator(pool, customSchema)
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Add a job with custom schema
	utils := worker.NewWorkerUtils(pool, customSchema)
	_, err = utils.QuickAddJob(ctx, "schema_test", map[string]interface{}{"schema": customSchema})
	require.NoError(t, err)

	var processedData map[string]interface{}

	options := worker.RunnerOptions{
		TaskList: map[string]worker.TaskHandler{
			"schema_test": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
				return json.Unmarshal(payload, &processedData)
			},
		},
		PgPool: pool,
		Schema: customSchema,
	}

	err = worker.RunOnce(ctx, options)
	require.NoError(t, err, "RunOnce should work with custom schema")

	require.NotNil(t, processedData, "Should have processed job data")
	require.Equal(t, customSchema, processedData["schema"], "Should have processed correct schema data")
}
