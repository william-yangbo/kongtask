package sql

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FailJob marks a job as failed using the fail_job database function
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
	query := fmt.Sprintf("SELECT FROM %s.fail_job($1, $2, $3)", compiledSharedOptions.EscapedWorkerSchema)

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
