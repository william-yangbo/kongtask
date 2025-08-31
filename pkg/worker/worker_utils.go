package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// WorkerUtils provides utilities for interacting with graphile-worker (v0.4.0 addition)
type WorkerUtils struct {
	pool   *pgxpool.Pool
	schema string
	logger *logger.Logger
	// For connection string based utils that need cleanup
	ownedPool bool
}

// WorkerUtilsOptions mirrors graphile-worker's WorkerUtilsOptions
type WorkerUtilsOptions struct {
	ConnectionString string
	PgPool           *pgxpool.Pool
	Schema           string
	Logger           *logger.Logger
}

// NewWorkerUtils creates a new WorkerUtils instance (v0.4.0 makeWorkerUtils equivalent)
func NewWorkerUtils(pool *pgxpool.Pool, schema string) *WorkerUtils {
	return &WorkerUtils{
		pool:      pool,
		schema:    schema,
		logger:    logger.DefaultLogger.Scope(logger.LogScope{Label: "WorkerUtils"}),
		ownedPool: false,
	}
}

// MakeWorkerUtils creates a new WorkerUtils instance from options (v0.4.0 makeWorkerUtils equivalent)
func MakeWorkerUtils(ctx context.Context, options WorkerUtilsOptions) (*WorkerUtils, error) {
	var pool *pgxpool.Pool
	var ownedPool bool

	baseLogger := options.Logger
	if baseLogger == nil {
		baseLogger = logger.DefaultLogger
	}

	workLogger := baseLogger.Scope(logger.LogScope{Label: "WorkerUtils"})

	schema := options.Schema
	if schema == "" {
		schema = "graphile_worker"
	}

	if options.PgPool != nil {
		pool = options.PgPool
		ownedPool = false
	} else if options.ConnectionString != "" {
		var err error
		pool, err = pgxpool.New(ctx, options.ConnectionString)
		if err != nil {
			return nil, fmt.Errorf("failed to create connection pool: %w", err)
		}
		ownedPool = true
	} else {
		return nil, fmt.Errorf("either PgPool or ConnectionString must be provided")
	}

	// Auto-migrate like graphile-worker does
	migrator := migrate.NewMigrator(pool, schema)
	if err := migrator.Migrate(ctx); err != nil {
		if ownedPool {
			pool.Close()
		}
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &WorkerUtils{
		pool:      pool,
		schema:    schema,
		logger:    workLogger,
		ownedPool: ownedPool,
	}, nil
}

// QuickAddJob is a utility for quickly adding jobs (v0.4.0 quickAddJob equivalent)
func (wu *WorkerUtils) QuickAddJob(ctx context.Context, taskIdentifier string, payload interface{}, spec ...TaskSpec) (string, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	conn, err := wu.pool.Acquire(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	var jobID string

	if len(spec) == 0 {
		// Simple case - use the 2-parameter add_job function
		query := fmt.Sprintf("SELECT %s.add_job($1, $2)", wu.schema)
		err = conn.QueryRow(ctx, query, taskIdentifier, string(payloadJSON)).Scan(&jobID)
	} else {
		// Complex case with TaskSpec
		s := spec[0]

		if s.JobKey != nil {
			// Use 6-parameter add_job with job_key (v0.4.0)
			query := fmt.Sprintf("SELECT %s.add_job($1, $2, $3, $4, $5, $6)", wu.schema)

			queueName := ""
			if s.QueueName != nil {
				queueName = *s.QueueName
			}

			runAt := time.Now()
			if s.RunAt != nil {
				runAt = *s.RunAt
			}

			maxAttempts := 25
			if s.MaxAttempts != nil {
				maxAttempts = *s.MaxAttempts
			}

			err = conn.QueryRow(ctx, query, taskIdentifier, string(payloadJSON), queueName, runAt, maxAttempts, *s.JobKey).Scan(&jobID)
		} else {
			// Use 5-parameter add_job (legacy format)
			queueName := ""
			if s.QueueName != nil {
				queueName = *s.QueueName
			}

			runAt := time.Now()
			if s.RunAt != nil {
				runAt = *s.RunAt
			}

			maxAttempts := 25
			if s.MaxAttempts != nil {
				maxAttempts = *s.MaxAttempts
			}

			query := fmt.Sprintf("SELECT %s.add_job($1, $2, $3, $4, $5)", wu.schema)
			err = conn.QueryRow(ctx, query, taskIdentifier, string(payloadJSON), queueName, runAt, maxAttempts).Scan(&jobID)
		}
	}

	if err != nil {
		return "", fmt.Errorf("failed to add job: %w", err)
	}

	return jobID, nil
}

// MigrateDatabase runs database migrations (v0.4.0 migrate equivalent)
func (wu *WorkerUtils) MigrateDatabase(ctx context.Context) error {
	migrator := migrate.NewMigrator(wu.pool, wu.schema)
	return migrator.Migrate(ctx)
}

// Release releases the connection pool if it's owned by this WorkerUtils instance
// This matches the graphile-worker utils.release() method
func (wu *WorkerUtils) Release() error {
	if wu.ownedPool && wu.pool != nil {
		wu.pool.Close()
		wu.pool = nil
	}
	return nil
}

// WithPgClient provides access to a database connection (v0.4.0 withPgClient equivalent)
func (wu *WorkerUtils) WithPgClient(ctx context.Context, fn func(conn *pgxpool.Conn) error) error {
	conn, err := wu.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	return fn(conn)
}

// Logger returns the logger instance
func (wu *WorkerUtils) Logger() *logger.Logger {
	return wu.logger
}

// GetJobByKey retrieves a job by its key (v0.4.0 feature)
func (wu *WorkerUtils) GetJobByKey(ctx context.Context, jobKey string) (*Job, error) {
	conn, err := wu.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	query := fmt.Sprintf("SELECT id, queue_name, task_identifier, payload, run_at, attempts, max_attempts, last_error, created_at, updated_at, key, locked_at, locked_by FROM %s.jobs WHERE key = $1 LIMIT 1", wu.schema)

	var job Job
	var queueName, lastError, key, lockedBy *string
	var lockedAt *time.Time
	var id int

	err = conn.QueryRow(ctx, query, jobKey).Scan(
		&id, &queueName, &job.TaskIdentifier, &job.Payload,
		&job.RunAt, &job.AttemptCount, &job.MaxAttempts,
		&lastError, &job.CreatedAt, &job.UpdatedAt,
		&key, &lockedAt, &lockedBy,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get job by key: %w", err)
	}

	// Convert int id to string for v0.4.0 compatibility
	job.ID = fmt.Sprintf("%d", id)
	job.QueueName = queueName
	job.LastError = lastError
	job.Key = key
	job.LockedAt = lockedAt
	job.LockedBy = lockedBy

	return &job, nil
}

// QuickAddJobGlobal is a global function for quickly adding a job (v0.4.0 quickAddJob equivalent)
// This function can be used to quickly add a job; however if you need to call
// this more than once in your process you should instead create a WorkerUtils
// instance for efficiency and performance sake.
func QuickAddJobGlobal(ctx context.Context, options WorkerUtilsOptions, taskIdentifier string, payload interface{}, spec ...TaskSpec) (string, error) {
	utils, err := MakeWorkerUtils(ctx, options)
	if err != nil {
		return "", err
	}
	defer func() { _ = utils.Release() }() // Ignore release error

	return utils.QuickAddJob(ctx, taskIdentifier, payload, spec...)
}
