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

			// Use the same SQL query as TypeScript version
			query := `
				SELECT * FROM graphile_worker.add_job(
					identifier => $1::text,
					payload => $2::json,
					queue_name => coalesce($3::text, public.gen_random_uuid()::text),
					run_at => coalesce($4::timestamptz, now()),
					max_attempts => coalesce($5::int, 25),
					job_key => $6::text
				)
			`

			// Prepare parameters exactly like TypeScript
			var queueName, runAtStr, jobKey *string
			var maxAttempts *int

			if spec.QueueName != nil {
				queueName = spec.QueueName
			}

			if spec.RunAt != nil {
				runAtStr = &[]string{spec.RunAt.Format("2006-01-02T15:04:05Z07:00")}[0]
			}

			if spec.MaxAttempts != nil {
				maxAttempts = spec.MaxAttempts
			}

			if spec.JobKey != nil {
				jobKey = spec.JobKey
			}

			_, err = tx.Exec(ctx, query, identifier, string(payloadJSON), queueName, runAtStr, maxAttempts, jobKey)
			return err
		})
	}
}

// MakeJobHelpers creates a JobHelpers instance for a job
// This mirrors the TypeScript makeJobHelpers function exactly
func MakeJobHelpers(job *Job, withPgClient WithPgClient, baseLogger *logger.Logger) *JobHelpers {
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

	// Query helper - mirrors TypeScript query function
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

	// AddJob helper using MakeAddJob
	helpers.AddJob = MakeAddJob(withPgClient)

	// TODO: Add deprecated debug method warning like TypeScript
	// This would require extending the JobHelpers struct or using reflection

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
		defer tx.Rollback(ctx)

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
		defer tx.Rollback(ctx)

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
