package sql

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CompleteJob marks a job as completed using the complete_job database function
func CompleteJob(
	ctx context.Context,
	compiledSharedOptions CompiledSharedOptions,
	pool *pgxpool.Pool,
	workerId string,
	jobId string,
) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	// TODO: retry logic, in case of server connection interruption
	query := fmt.Sprintf("SELECT FROM %s.complete_job($1, $2)", compiledSharedOptions.EscapedWorkerSchema)

	if compiledSharedOptions.NoPreparedStatements {
		// Use simple protocol to avoid prepared statements (for pgBouncer compatibility)
		_, err = conn.Exec(ctx, query, pgx.QueryExecModeSimpleProtocol, workerId, jobId)
	} else {
		_, err = conn.Exec(ctx, query, workerId, jobId)
	}

	if err != nil {
		return fmt.Errorf("failed to complete job: %w", err)
	}

	return nil
}
