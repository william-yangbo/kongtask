package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// JobHelpers provides utility functions for task handlers (renamed from Helpers in v0.4.0)
type JobHelpers struct {
	Logger       *logger.Logger
	Job          *Job
	WithPgClient func(ctx context.Context, fn func(pgx.Tx) error) error
	Query        func(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	AddJob       func(ctx context.Context, taskIdentifier string, payload interface{}, spec ...TaskSpec) error
	// Debug is a deprecated method that mirrors TypeScript's deprecated debug functionality
	Debug func(format string, parameters ...interface{})
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

	// WithPgClient helper - enhanced with connection error handling from graphile-worker commit 9d0362c
	helpers.WithPgClient = func(ctx context.Context, fn func(pgx.Tx) error) error {
		return withPgClientErrorHandling(w.pool, w.logger, ctx, func(conn *pgxpool.Conn) error {
			tx, err := conn.Begin(ctx)
			if err != nil {
				return fmt.Errorf("failed to begin transaction: %w", err)
			}
			defer func() { _ = tx.Rollback(ctx) }() // Ignore rollback error in defer

			if err := fn(tx); err != nil {
				return err
			}

			return tx.Commit(ctx)
		})
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

	// DEPRECATED debug method that mirrors TypeScript functionality
	helpers.Debug = func(format string, parameters ...interface{}) {
		helpers.Logger.Error("REMOVED: `helpers.Debug` has been replaced with `helpers.Logger.Debug`; please do not use `helpers.Debug`")
		helpers.Logger.Debug(format, map[string]interface{}{"parameters": parameters})
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
		// Complex case with TaskSpec - use the 9-parameter add_job with job_key_mode (commit e7ab91e)
		spec := specs[0]

		// Set default values as per graphile-worker
		var queueName *string
		if spec.QueueName != nil {
			queueName = spec.QueueName
		}

		var runAt *time.Time
		if spec.RunAt != nil {
			runAt = spec.RunAt
		}

		var maxAttempts *int
		if spec.MaxAttempts != nil {
			maxAttempts = spec.MaxAttempts
		}

		var priority *int
		if spec.Priority != nil {
			priority = spec.Priority
		}

		var jobKeyMode *string
		if spec.JobKeyMode != nil {
			jobKeyMode = spec.JobKeyMode
		}

		// Use 9-parameter add_job function (commit e7ab91e format)
		query := fmt.Sprintf("SELECT %s.add_job($1, $2, $3, $4, $5, $6, $7, $8, $9)", w.schema)
		_, err = conn.Exec(ctx, query,
			taskIdentifier,      // identifier
			string(payloadJSON), // payload
			queueName,           // queue_name
			runAt,               // run_at
			maxAttempts,         // max_attempts
			spec.JobKey,         // job_key
			priority,            // priority
			spec.Flags,          // flags
			jobKeyMode,          // job_key_mode
		)
	}

	if err != nil {
		return fmt.Errorf("failed to add job: %w", err)
	}

	return nil
}

// AddJobWithReplace adds a job with replace mode (debouncing behavior)
// This is a convenience function for the most common job key usage pattern
func (w *Worker) AddJobWithReplace(ctx context.Context, taskIdentifier string, payload interface{}, jobKey string, spec ...TaskSpec) error {
	var s TaskSpec
	if len(spec) > 0 {
		s = spec[0]
	}
	s.JobKey = &jobKey
	replaceMode := JobKeyModeReplace
	s.JobKeyMode = &replaceMode
	return w.AddJobWithTaskSpec(ctx, taskIdentifier, payload, s)
}

// AddJobWithPreserveRunAt adds a job with preserve_run_at mode (throttling behavior)
// This preserves the original run_at time when updating an existing job
func (w *Worker) AddJobWithPreserveRunAt(ctx context.Context, taskIdentifier string, payload interface{}, jobKey string, spec ...TaskSpec) error {
	var s TaskSpec
	if len(spec) > 0 {
		s = spec[0]
	}
	s.JobKey = &jobKey
	preserveMode := JobKeyModePreserveRunAt
	s.JobKeyMode = &preserveMode
	return w.AddJobWithTaskSpec(ctx, taskIdentifier, payload, s)
}

// AddJobWithUnsafeDedupe adds a job with unsafe_dedupe mode (dangerous - use with caution!)
// This mode will not update existing jobs even if they are locked or failed
func (w *Worker) AddJobWithUnsafeDedupe(ctx context.Context, taskIdentifier string, payload interface{}, jobKey string, spec ...TaskSpec) error {
	var s TaskSpec
	if len(spec) > 0 {
		s = spec[0]
	}
	s.JobKey = &jobKey
	dedupeMode := JobKeyModeUnsafeDedupe
	s.JobKeyMode = &dedupeMode
	return w.AddJobWithTaskSpec(ctx, taskIdentifier, payload, s)
}
