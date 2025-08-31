package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// JobHelpers provides utility functions for task handlers (renamed from Helpers in v0.4.0)
type JobHelpers struct {
	Logger       *logger.Logger
	Job          *Job
	WithPgClient func(ctx context.Context, fn func(pgx.Tx) error) error
	Query        func(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	AddJob       func(ctx context.Context, taskIdentifier string, payload interface{}, spec ...TaskSpec) error
}

// Helpers is an alias for backward compatibility
type Helpers = JobHelpers

// JobOptions represents options for adding a job (deprecated: use TaskSpec in v0.4.0)
type JobOptions struct {
	QueueName   *string `json:"queue_name,omitempty"`
	RunAt       *string `json:"run_at,omitempty"` // ISO 8601 timestamp
	MaxAttempts *int    `json:"max_attempts,omitempty"`
}

// CreateHelpers creates a new Helpers instance for a job
func (w *Worker) CreateHelpers(ctx context.Context, job *Job) *Helpers {
	// Create scoped logger
	scopedLogger := w.logger.Scope(logger.LogScope{
		Label:          "job",
		WorkerID:       w.workerID,
		TaskIdentifier: job.TaskIdentifier,
		JobID:          &job.ID,
	})

	helpers := &Helpers{
		Logger: scopedLogger,
		Job:    job,
	}

	// WithPgClient helper
	helpers.WithPgClient = func(ctx context.Context, fn func(pgx.Tx) error) error {
		conn, err := w.pool.Acquire(ctx)
		if err != nil {
			return fmt.Errorf("failed to acquire connection: %w", err)
		}
		defer conn.Release()

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }() // Ignore rollback error in defer

		if err := fn(tx); err != nil {
			return err
		}

		return tx.Commit(ctx)
	}

	// Query helper - convenience wrapper for database queries
	helpers.Query = func(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
		conn, err := w.pool.Acquire(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to acquire connection: %w", err)
		}
		defer conn.Release()

		return conn.Query(ctx, sql, args...)
	}

	// AddJob helper (v0.4.0: now uses TaskSpec)
	helpers.AddJob = func(ctx context.Context, taskIdentifier string, payload interface{}, spec ...TaskSpec) error {
		return w.AddJobWithTaskSpec(ctx, taskIdentifier, payload, spec...)
	}

	return helpers
}

// AddJobWithOptions adds a job with optional parameters (enhanced version for v0.2.0)
func (w *Worker) AddJobWithOptions(ctx context.Context, taskIdentifier string, payload interface{}, options ...JobOptions) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	if len(options) == 0 {
		// Simple case - use the 2-parameter function
		query := fmt.Sprintf("SELECT %s.add_job($1, $2)", w.schema)
		_, err = conn.Exec(ctx, query, taskIdentifier, string(payloadJSON))
	} else {
		// Complex case with options - use the 5-parameter function (no priority in v0.2.0)
		opt := options[0]

		// Set defaults for optional parameters
		queueName := "public.gen_random_uuid()::text"
		if opt.QueueName != nil {
			queueName = *opt.QueueName
		}

		runAt := "now()"
		if opt.RunAt != nil {
			runAt = fmt.Sprintf("'%s'::timestamptz", *opt.RunAt)
		}

		maxAttempts := 25
		if opt.MaxAttempts != nil {
			maxAttempts = *opt.MaxAttempts
		}

		query := fmt.Sprintf("SELECT %s.add_job($1, $2, %s, %s, $3)", w.schema, queueName, runAt)
		_, err = conn.Exec(ctx, query, taskIdentifier, string(payloadJSON), maxAttempts)
	}

	if err != nil {
		return fmt.Errorf("failed to add job: %w", err)
	}

	return nil
}

// AddJobWithTaskSpec adds a job with TaskSpec options (v0.4.0 compatible)
func (w *Worker) AddJobWithTaskSpec(ctx context.Context, taskIdentifier string, payload interface{}, specs ...TaskSpec) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	if len(specs) == 0 {
		// Simple case - use the 2-parameter add_job function
		query := fmt.Sprintf("SELECT %s.add_job($1, $2)", w.schema)
		_, err = conn.Exec(ctx, query, taskIdentifier, string(payloadJSON))
	} else {
		// Complex case with TaskSpec - use the enhanced add_job function (v0.4.0 with job_key)
		spec := specs[0]

		// Build query based on available parameters
		if spec.JobKey != nil {
			// Use 6-parameter add_job with job_key (v0.4.0)
			query := fmt.Sprintf("SELECT %s.add_job($1, $2, $3, $4, $5, $6)", w.schema)

			queueName := ""
			if spec.QueueName != nil {
				queueName = *spec.QueueName
			}

			runAt := "now()"
			if spec.RunAt != nil {
				runAt = spec.RunAt.Format("2006-01-02T15:04:05Z07:00")
			}

			maxAttempts := 25
			if spec.MaxAttempts != nil {
				maxAttempts = *spec.MaxAttempts
			}

			_, err = conn.Exec(ctx, query, taskIdentifier, string(payloadJSON), queueName, runAt, maxAttempts, *spec.JobKey)
		} else {
			// Use 5-parameter add_job (legacy format)
			queueName := "public.gen_random_uuid()::text"
			if spec.QueueName != nil {
				queueName = fmt.Sprintf("'%s'", *spec.QueueName)
			}

			runAt := "now()"
			if spec.RunAt != nil {
				runAt = fmt.Sprintf("'%s'::timestamptz", spec.RunAt.Format("2006-01-02T15:04:05Z07:00"))
			}

			maxAttempts := 25
			if spec.MaxAttempts != nil {
				maxAttempts = *spec.MaxAttempts
			}

			query := fmt.Sprintf("SELECT %s.add_job($1, $2, %s, %s, $3)", w.schema, queueName, runAt)
			_, err = conn.Exec(ctx, query, taskIdentifier, string(payloadJSON), maxAttempts)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to add job: %w", err)
	}

	return nil
}
