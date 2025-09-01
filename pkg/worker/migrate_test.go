package worker

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMigrate(t *testing.T) {
	ctx := context.Background()

	// Start PostgreSQL container
	postgresContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2)),
	)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, postgresContainer.Terminate(ctx))
	}()

	// Get connection string
	connectionString, err := postgresContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Create connection pool
	pool, err := pgxpool.New(ctx, connectionString)
	require.NoError(t, err)
	defer pool.Close()

	// Test with default options
	t.Run("DefaultOptions", func(t *testing.T) {
		options := WorkerSharedOptions{}
		err := Migrate(ctx, options, pool)
		assert.NoError(t, err)

		// Verify schema was created
		var schemaExists bool
		err = pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = 'graphile_worker')").
			Scan(&schemaExists)
		assert.NoError(t, err)
		assert.True(t, schemaExists)

		// Verify migrations table exists
		var tableExists bool
		err = pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'graphile_worker' AND table_name = 'migrations')").
			Scan(&tableExists)
		assert.NoError(t, err)
		assert.True(t, tableExists)
	})

	// Test with environment variable
	t.Run("WithEnvSchema", func(t *testing.T) {
		// Clean up default schema first
		_, err = pool.Exec(ctx, "DROP SCHEMA IF EXISTS graphile_worker CASCADE")
		require.NoError(t, err)

		// Set environment variable
		if err := os.Setenv("GRAPHILE_WORKER_SCHEMA", "custom_worker"); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := os.Unsetenv("GRAPHILE_WORKER_SCHEMA"); err != nil {
				t.Errorf("Failed to unset environment variable: %v", err)
			}
		}()

		options := WorkerSharedOptions{}
		err := Migrate(ctx, options, pool)
		assert.NoError(t, err)

		// Verify custom schema was created
		var schemaExists bool
		err = pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = 'custom_worker')").
			Scan(&schemaExists)
		assert.NoError(t, err)
		assert.True(t, schemaExists)
	})
}

func TestMigrateWithSharedOptions(t *testing.T) {
	ctx := context.Background()

	// Start PostgreSQL container
	postgresContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2)),
	)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, postgresContainer.Terminate(ctx))
	}()

	// Get connection string
	connectionString, err := postgresContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Create connection pool
	pool, err := pgxpool.New(ctx, connectionString)
	require.NoError(t, err)
	defer pool.Close()

	// Test with custom schema in options
	customSchema := "my_custom_schema"
	options := SharedOptions{
		Schema: &customSchema,
	}

	err = MigrateWithSharedOptions(ctx, options, pool)
	assert.NoError(t, err)

	// Verify custom schema was created
	var schemaExists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = 'my_custom_schema')").
		Scan(&schemaExists)
	assert.NoError(t, err)
	assert.True(t, schemaExists)
}
