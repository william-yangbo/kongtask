package sql

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FailJob marks a job as failed using inline SQL (moved from database function)
func FailJob(
	ctx context.Context,
	compiledSharedOptions CompiledSharedOptions,
	pool *pgxpool.Pool,
	workerId string,
	jobId string,
	message string,
) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	// TODO: retry logic, in case of server connection interruption
	query := fmt.Sprintf(`with j as (
update %s.jobs
set
last_error = $3,
run_at = greatest(now(), run_at) + (exp(least(attempts, 10))::text || ' seconds')::interval,
locked_by = null,
locked_at = null
where id = $2 and locked_by = $1
returning *
)
update %s.job_queues
set locked_by = null, locked_at = null
from j
where job_queues.queue_name = j.queue_name and job_queues.locked_by = $1;`,
		compiledSharedOptions.EscapedWorkerSchema,
		compiledSharedOptions.EscapedWorkerSchema)

	if compiledSharedOptions.NoPreparedStatements {
		// Use simple protocol to avoid prepared statements (for pgBouncer compatibility)
		_, err = conn.Exec(ctx, query, pgx.QueryExecModeSimpleProtocol, workerId, jobId, message)
	} else {
		_, err = conn.Exec(ctx, query, workerId, jobId, message)
	}

	if err != nil {
		return fmt.Errorf("failed to fail job: %w", err)
	}

	return nil
}
