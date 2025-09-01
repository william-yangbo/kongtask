package integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/internal/testutil"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// TestCustomSchema tests using a custom database schema (equivalent to altschema CI job)
// This test mirrors the TypeScript altschema test which sets GRAPHILE_WORKER_SCHEMA=custom_schema
func TestCustomSchema(t *testing.T) {
	// Set custom schema environment variable (equivalent to CI altschema job)
	originalSchema := os.Getenv("GRAPHILE_WORKER_SCHEMA")
	os.Setenv("GRAPHILE_WORKER_SCHEMA", "custom_schema")
	defer func() {
		if originalSchema != "" {
			os.Setenv("GRAPHILE_WORKER_SCHEMA", originalSchema)
		} else {
			os.Unsetenv("GRAPHILE_WORKER_SCHEMA")
		}
	}()

	_, pool := testutil.StartPostgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Log("🔧 Testing custom schema functionality...")

	// Test 1: Migrator should use custom schema from environment variable
	migrator := migrate.NewMigrator(pool, "") // Empty string should use env var
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// Verify custom schema was created
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var schemaExists bool
	err = conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = 'custom_schema')").
		Scan(&schemaExists)
	require.NoError(t, err)
	require.True(t, schemaExists, "Custom schema 'custom_schema' should exist")

	// Verify tables were created in custom schema
	var tableExists bool
	err = conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'custom_schema' AND table_name = 'jobs')").
		Scan(&tableExists)
	require.NoError(t, err)
	require.True(t, tableExists, "Jobs table should exist in custom schema")

	t.Log("✅ Custom schema migration successful")

	// Test 2: WorkerUtils should work with custom schema
	workerUtils, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: "custom_schema", // Explicitly use custom schema
	})
	require.NoError(t, err)

	// Add a job using custom schema
	jobPayload := map[string]interface{}{
		"message": "test job in custom schema",
		"value":   42,
	}
	payloadBytes, err := json.Marshal(jobPayload)
	require.NoError(t, err)

	jobID, err := workerUtils.QuickAddJob(ctx, "test_task", json.RawMessage(payloadBytes), worker.TaskSpec{})
	require.NoError(t, err)
	require.NotEqual(t, 0, jobID)

	// Verify job was added to custom schema
	var jobCount int
	err = conn.QueryRow(ctx, "SELECT COUNT(*)::int FROM custom_schema.jobs").Scan(&jobCount)
	require.NoError(t, err)
	require.Equal(t, 1, jobCount, "Job should be in custom schema")

	t.Log("✅ WorkerUtils works with custom schema")

	// Test 3: Worker should process jobs from custom schema
	var processed bool
	tasks := map[string]worker.TaskHandler{
		"test_task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			var data map[string]interface{}
			err := json.Unmarshal(payload, &data)
			require.NoError(t, err)
			require.Equal(t, "test job in custom schema", data["message"])
			require.Equal(t, float64(42), data["value"])
			processed = true
			return nil
		},
	}

	options := worker.WorkerPoolOptions{
		Concurrency: 1,
		Schema:      "custom_schema", // Use custom schema
	}

	workerPool, err := worker.RunTaskList(ctx, options, tasks, pool)
	require.NoError(t, err)

	// Wait for job to be processed
	testutil.WaitForJobCount(t, pool, "custom_schema", 0, 10*time.Second)
	require.True(t, processed, "Job should have been processed")

	err = workerPool.Release()
	require.NoError(t, err)

	t.Log("✅ Worker processes jobs from custom schema")

	// Test 4: Test environment variable fallback without explicit schema
	workerUtils2, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		// No Schema specified - should use GRAPHILE_WORKER_SCHEMA env var
	})
	require.NoError(t, err)

	// Add another job (should go to custom_schema due to env var)
	jobID2, err := workerUtils2.QuickAddJob(ctx, "test_task2", json.RawMessage(`{"env_test": true}`), worker.TaskSpec{})
	require.NoError(t, err)
	require.NotEqual(t, 0, jobID2)

	// Verify job was added to custom schema (not default)
	err = conn.QueryRow(ctx, "SELECT COUNT(*)::int FROM custom_schema.jobs").Scan(&jobCount)
	require.NoError(t, err)
	require.Equal(t, 1, jobCount, "Second job should be in custom schema")

	// Verify no jobs in default schema tables (should not exist)
	var defaultSchemaExists bool
	err = conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = 'graphile_worker')").Scan(&defaultSchemaExists)
	require.NoError(t, err)
	require.False(t, defaultSchemaExists, "Default schema should not exist when using custom schema")

	t.Log("✅ Environment variable fallback works correctly")
}

// TestMultipleSchemas tests that different schema configurations work independently
func TestMultipleSchemas(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Log("🔧 Testing multiple schema isolation...")

	// Create two different schemas
	schema1 := "worker_schema_1"
	schema2 := "worker_schema_2"

	// Initialize both schemas
	migrator1 := migrate.NewMigrator(pool, schema1)
	err := migrator1.Migrate(ctx)
	require.NoError(t, err)

	migrator2 := migrate.NewMigrator(pool, schema2)
	err = migrator2.Migrate(ctx)
	require.NoError(t, err)

	// Create WorkerUtils for each schema
	workerUtils1, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: schema1,
	})
	require.NoError(t, err)

	workerUtils2, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: schema2,
	})
	require.NoError(t, err)

	// Add jobs to each schema
	_, err = workerUtils1.QuickAddJob(ctx, "task1", json.RawMessage(`{"schema": 1}`), worker.TaskSpec{})
	require.NoError(t, err)

	_, err = workerUtils2.QuickAddJob(ctx, "task2", json.RawMessage(`{"schema": 2}`), worker.TaskSpec{})
	require.NoError(t, err)

	// Verify job isolation
	count1 := testutil.JobCount(t, pool, schema1)
	count2 := testutil.JobCount(t, pool, schema2)

	require.Equal(t, 1, count1, "Schema 1 should have 1 job")
	require.Equal(t, 1, count2, "Schema 2 should have 1 job")

	// Process jobs with schema-specific workers
	var processed1, processed2 bool

	tasks1 := map[string]worker.TaskHandler{
		"task1": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			var data map[string]interface{}
			err := json.Unmarshal(payload, &data)
			require.NoError(t, err)
			require.Equal(t, float64(1), data["schema"])
			processed1 = true
			return nil
		},
	}

	tasks2 := map[string]worker.TaskHandler{
		"task2": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			var data map[string]interface{}
			err := json.Unmarshal(payload, &data)
			require.NoError(t, err)
			require.Equal(t, float64(2), data["schema"])
			processed2 = true
			return nil
		},
	}

	// Start workers for each schema
	worker1, err := worker.RunTaskList(ctx, worker.WorkerPoolOptions{
		Concurrency: 1,
		Schema:      schema1,
	}, tasks1, pool)
	require.NoError(t, err)

	worker2, err := worker.RunTaskList(ctx, worker.WorkerPoolOptions{
		Concurrency: 1,
		Schema:      schema2,
	}, tasks2, pool)
	require.NoError(t, err)

	// Wait for processing
	testutil.WaitForJobCount(t, pool, schema1, 0, 10*time.Second)
	testutil.WaitForJobCount(t, pool, schema2, 0, 10*time.Second)

	require.True(t, processed1, "Schema 1 job should be processed")
	require.True(t, processed2, "Schema 2 job should be processed")

	// Cleanup
	err = worker1.Release()
	require.NoError(t, err)
	err = worker2.Release()
	require.NoError(t, err)

	t.Log("✅ Multiple schema isolation works correctly")
}

// TestSchemaConfigurationPriority tests the priority order of schema configuration
func TestSchemaConfigurationPriority(t *testing.T) {
	_, pool := testutil.StartPostgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Log("🔧 Testing schema configuration priority...")

	// Set environment variable
	originalSchema := os.Getenv("GRAPHILE_WORKER_SCHEMA")
	os.Setenv("GRAPHILE_WORKER_SCHEMA", "env_schema")
	defer func() {
		if originalSchema != "" {
			os.Setenv("GRAPHILE_WORKER_SCHEMA", originalSchema)
		} else {
			os.Unsetenv("GRAPHILE_WORKER_SCHEMA")
		}
	}()

	// Test 1: Explicit schema parameter should override environment variable
	migrator := migrate.NewMigrator(pool, "explicit_schema")
	err := migrator.Migrate(ctx)
	require.NoError(t, err)

	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var schemaExists bool
	err = conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = 'explicit_schema')").
		Scan(&schemaExists)
	require.NoError(t, err)
	require.True(t, schemaExists, "Explicit schema should be created")

	// Test 2: Empty string should use environment variable
	migrator2 := migrate.NewMigrator(pool, "")
	err = migrator2.Migrate(ctx)
	require.NoError(t, err)

	err = conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = 'env_schema')").
		Scan(&schemaExists)
	require.NoError(t, err)
	require.True(t, schemaExists, "Environment schema should be created")

	// Test 3: WorkerUtils should follow same priority
	workerUtils, err := worker.MakeWorkerUtils(ctx, worker.WorkerUtilsOptions{
		PgPool: pool,
		Schema: "explicit_worker_schema",
	})
	require.NoError(t, err)

	// Initialize this schema first
	migrator3 := migrate.NewMigrator(pool, "explicit_worker_schema")
	err = migrator3.Migrate(ctx)
	require.NoError(t, err)

	// Add job with explicit schema
	_, err = workerUtils.QuickAddJob(ctx, "test", json.RawMessage(`{}`), worker.TaskSpec{})
	require.NoError(t, err)

	// Verify job is in explicit schema, not env schema
	var jobCount int
	err = conn.QueryRow(ctx, "SELECT COUNT(*)::int FROM explicit_worker_schema.jobs").Scan(&jobCount)
	require.NoError(t, err)
	require.Equal(t, 1, jobCount, "Job should be in explicit schema")

	err = conn.QueryRow(ctx, "SELECT COUNT(*)::int FROM env_schema.jobs").Scan(&jobCount)
	require.NoError(t, err)
	require.Equal(t, 0, jobCount, "Job should not be in env schema")

	t.Log("✅ Schema configuration priority works correctly")
}
