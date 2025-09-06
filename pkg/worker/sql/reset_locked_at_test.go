package sql

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	postgresContainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/william-yangbo/kongtask/internal/migrate"
)

func TestResetLockedAt(t *testing.T) {
	ctx := context.Background()

	// Start PostgreSQL container
	pgContainer, err := postgresContainer.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		postgresContainer.WithDatabase("testdb"),
		postgresContainer.WithUsername("testuser"),
		postgresContainer.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(30*time.Second)),
	)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, pgContainer.Terminate(ctx))
	}()

	// Get connection string
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Create connection pool for migrations
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	defer pool.Close()

	// Run migrations to set up tables
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	// Connect to database for testing
	conn, err := pgx.Connect(ctx, connStr)
	require.NoError(t, err)
	defer conn.Close(ctx)

	// Insert a test job that's been locked for more than 4 hours
	longAgo := time.Now().Add(-5 * time.Hour)
	_, err = conn.Exec(ctx, `
		INSERT INTO graphile_worker.jobs (id, queue_name, task_identifier, payload, locked_at, locked_by)
		VALUES (1, 'test_queue', 'test_task', '{}', $1, 'old_worker')
	`, longAgo)
	require.NoError(t, err)

	// Update the job queue that was automatically created to have an old lock
	_, err = conn.Exec(ctx, `
		UPDATE graphile_worker.job_queues 
		SET locked_at = $1, locked_by = 'old_worker'
		WHERE queue_name = 'test_queue'
	`, longAgo)
	require.NoError(t, err)

	// Verify jobs are initially locked
	var lockedAt *time.Time
	var lockedBy *string
	err = conn.QueryRow(ctx, "SELECT locked_at, locked_by FROM graphile_worker.jobs WHERE id = 1").Scan(&lockedAt, &lockedBy)
	require.NoError(t, err)
	assert.NotNil(t, lockedAt)
	assert.NotNil(t, lockedBy)

	// Verify queue is initially locked
	err = conn.QueryRow(ctx, "SELECT locked_at, locked_by FROM graphile_worker.job_queues WHERE queue_name = 'test_queue'").Scan(&lockedAt, &lockedBy)
	require.NoError(t, err)
	assert.NotNil(t, lockedAt)
	assert.NotNil(t, lockedBy)

	// Test resetLockedAt function
	sharedOptions := ResetLockedSharedOptions{
		EscapedWorkerSchema:  "\"graphile_worker\"",
		WorkerSchema:         "graphile_worker",
		NoPreparedStatements: true,
		UseNodeTime:          false, // Use PostgreSQL time
	}

	err = ResetLockedAt(ctx, sharedOptions, conn, nil)
	require.NoError(t, err)

	// Verify job lock was cleared
	err = conn.QueryRow(ctx, "SELECT locked_at, locked_by FROM graphile_worker.jobs WHERE id = 1").Scan(&lockedAt, &lockedBy)
	require.NoError(t, err)
	assert.Nil(t, lockedAt)
	assert.Nil(t, lockedBy)

	// Verify queue lock was cleared
	err = conn.QueryRow(ctx, "SELECT locked_at, locked_by FROM graphile_worker.job_queues WHERE queue_name = 'test_queue'").Scan(&lockedAt, &lockedBy)
	require.NoError(t, err)
	assert.Nil(t, lockedAt)
	assert.Nil(t, lockedBy)
}

func TestResetLockedAtWithNodeTime(t *testing.T) {
	ctx := context.Background()

	// Start PostgreSQL container
	pgContainer, err := postgresContainer.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		postgresContainer.WithDatabase("testdb"),
		postgresContainer.WithUsername("testuser"),
		postgresContainer.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(30*time.Second)),
	)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, pgContainer.Terminate(ctx))
	}()

	// Get connection string
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Create connection pool for migrations
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	defer pool.Close()

	// Run migrations to set up tables
	migrator := migrate.NewMigrator(pool, "graphile_worker")
	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	// Connect to database for testing
	conn, err := pgx.Connect(ctx, connStr)
	require.NoError(t, err)
	defer conn.Close(ctx)

	// Insert a test job that's been locked for more than 4 hours
	longAgo := time.Now().Add(-5 * time.Hour)
	_, err = conn.Exec(ctx, `
		INSERT INTO graphile_worker.jobs (id, queue_name, task_identifier, payload, locked_at, locked_by)
		VALUES (1, 'test_queue', 'test_task', '{}', $1, 'old_worker')
	`, longAgo)
	require.NoError(t, err)

	// Test resetLockedAt function with Node time
	sharedOptions := ResetLockedSharedOptions{
		EscapedWorkerSchema:  "\"graphile_worker\"",
		WorkerSchema:         "graphile_worker",
		NoPreparedStatements: true,
		UseNodeTime:          true, // Use Node time
	}

	timeProvider := func() interface{} {
		return time.Now()
	}

	err = ResetLockedAt(ctx, sharedOptions, conn, timeProvider)
	require.NoError(t, err)

	// Verify job lock was cleared
	var lockedAt *time.Time
	var lockedBy *string
	err = conn.QueryRow(ctx, "SELECT locked_at, locked_by FROM graphile_worker.jobs WHERE id = 1").Scan(&lockedAt, &lockedBy)
	require.NoError(t, err)
	assert.Nil(t, lockedAt)
	assert.Nil(t, lockedBy)
}
