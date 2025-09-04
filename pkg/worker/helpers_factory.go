package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// MakeAddJob creates an AddJob function with the given WithPgClient function
// This mirrors the TypeScript makeAddJob function exactly
func MakeAddJob(withPgClient WithPgClient) AddJobFunction {
	return MakeAddJobWithOptions(nil, withPgClient)
}

// MakeAddJobWithOptions creates an AddJob function with options and WithPgClient function
// This mirrors the TypeScript makeAddJob function exactly with schema and useNodeTime support
func MakeAddJobWithOptions(options *SharedOptions, withPgClient WithPgClient) AddJobFunction {
	return func(ctx context.Context, identifier string, payload interface{}, specs ...TaskSpec) error {
		var spec TaskSpec
		if len(specs) > 0 {
			spec = specs[0]
		}

		return withPgClient(ctx, func(tx pgx.Tx) error {
			payloadJSON, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("failed to marshal payload: %w", err)
			}

			// Get escaped schema name, useNodeTime setting, and time provider from options
			schema := "graphile_worker"
			useNodeTime := false
			var timeProvider = NewRealTimeProvider() // Default to real time
			if options != nil {
				if options.Schema != nil {
					schema = *options.Schema
				}
				if options.UseNodeTime != nil {
					useNodeTime = *options.UseNodeTime
				}
				if options.TimeProvider != nil {
					timeProvider = options.TimeProvider
				}
			}
			// Simple identifier escaping (Go equivalent of Client.prototype.escapeIdentifier)
			escapedSchema := `"` + schema + `"`

			// Use the exact same SQL query as TypeScript version with 7 parameters (including priority)
			query := fmt.Sprintf(`
				SELECT * FROM %s.add_job(
					identifier => $1::text,
					payload => $2::json,
					queue_name => $3::text,
					run_at => $4::timestamptz,
					max_attempts => $5::int,
					job_key => $6::text,
					priority => $7::int
				)
			`, escapedSchema)

			// Prepare parameters exactly like TypeScript
			var queueName, runAtStr, jobKey *string
			var maxAttempts, priority *int

			if spec.QueueName != nil {
				queueName = spec.QueueName
			}

			// Handle runAt with useNodeTime logic (matches TypeScript helpers.ts exactly)
			if spec.RunAt != nil {
				// If there's an explicit run at, use that
				runAtStr = &[]string{spec.RunAt.Format("2006-01-02T15:04:05Z07:00")}[0]
			} else if useNodeTime {
				// If we've been told to use Node time, use the time provider (supports testing)
				runAtStr = &[]string{timeProvider.Now().Format("2006-01-02T15:04:05Z07:00")}[0]
			}
			// Otherwise pass nil and the function will use `now()` internally

			if spec.MaxAttempts != nil {
				maxAttempts = spec.MaxAttempts
			}

			if spec.JobKey != nil {
				jobKey = spec.JobKey
			}

			if spec.Priority != nil {
				priority = spec.Priority
			}

			_, err = tx.Exec(ctx, query, identifier, string(payloadJSON), queueName, runAtStr, maxAttempts, jobKey, priority)
			return err
		})
	}
}

// MakeJobHelpers creates a JobHelpers instance for a job
// This mirrors the TypeScript makeJobHelpers function exactly
func MakeJobHelpers(options *SharedOptions, job *Job, withPgClient WithPgClient, overrideLogger *logger.Logger) *JobHelpers {
	// Use override logger or get from processSharedOptions like TypeScript
	var baseLogger *logger.Logger
	if overrideLogger != nil {
		baseLogger = overrideLogger
	} else if options != nil && options.Logger != nil {
		baseLogger = options.Logger
	} else {
		// Default logger similar to processSharedOptions behavior
		baseLogger = logger.NewLogger(nil)
	}

	// Create scoped logger exactly like TypeScript
	jobLogger := baseLogger.Scope(logger.LogScope{
		Label:          "job",
		TaskIdentifier: job.TaskIdentifier,
		JobID:          &job.ID,
	})

	helpers := &JobHelpers{
		Job:          job,
		Logger:       jobLogger,
		WithPgClient: withPgClient,
	}

	// Query helper - mirrors TypeScript query function exactly
	helpers.Query = func(ctx context.Context, queryText string, args ...interface{}) (pgx.Rows, error) {
		var rows pgx.Rows
		var err error

		queryErr := withPgClient(ctx, func(tx pgx.Tx) error {
			rows, err = tx.Query(ctx, queryText, args...)
			return err
		})

		if queryErr != nil {
			return nil, queryErr
		}
		return rows, err
	}

	// AddJob helper using MakeAddJob - mirrors TypeScript exactly
	helpers.AddJob = MakeAddJobWithOptions(options, withPgClient)

	// DEPRECATED debug method that mirrors TypeScript Object.assign(helpers, { debug(...) })
	helpers.Debug = func(format string, parameters ...interface{}) {
		helpers.Logger.Error("REMOVED: `helpers.Debug` has been replaced with `helpers.Logger.Debug`; please do not use `helpers.Debug`")
		helpers.Logger.Debug(format, map[string]interface{}{"parameters": parameters})
	}

	return helpers
}

// MakeWithPgClientFromPool creates a WithPgClient function from a connection pool
// This mirrors the TypeScript makeWithPgClientFromPool function exactly
func MakeWithPgClientFromPool(pgPool *pgxpool.Pool) WithPgClient {
	return func(ctx context.Context, callback func(tx pgx.Tx) error) error {
		conn, err := pgPool.Acquire(ctx)
		if err != nil {
			return fmt.Errorf("failed to acquire connection: %w", err)
		}
		defer conn.Release()

		// Begin transaction like TypeScript version uses client
		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }() // Ignore rollback error in defer

		if err := callback(tx); err != nil {
			return err
		}

		return tx.Commit(ctx)
	}
}

// MakeWithPgClientFromClient creates a WithPgClient function from a single client
// This mirrors the TypeScript makeWithPgClientFromClient function exactly
func MakeWithPgClientFromClient(conn *pgxpool.Conn) WithPgClient {
	return func(ctx context.Context, callback func(tx pgx.Tx) error) error {
		// Begin transaction like TypeScript version
		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }() // Ignore rollback error in defer

		if err := callback(tx); err != nil {
			return err
		}

		return tx.Commit(ctx)
	}
}

// AddDeprecatedDebugMethod adds deprecated debug method to helpers
// This mirrors the TypeScript deprecated debug method
func AddDeprecatedDebugMethod(helpers *JobHelpers) {
	// Note: In Go, we can't dynamically add methods to structs like TypeScript's Object.assign
	// Instead, we can provide a warning through the logger when debug-like functionality is needed
	helpers.Logger.Debug("DEPRECATED: Use helpers.Logger.Debug instead of helpers.debug")
}
