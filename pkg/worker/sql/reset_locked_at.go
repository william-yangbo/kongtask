package sql

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CompiledSharedOptions represents the shared configuration
type ResetLockedSharedOptions struct {
	EscapedWorkerSchema  string
	WorkerSchema         string
	NoPreparedStatements bool
	UseNodeTime          bool
}

// ResetLockedAt clears locked_at and locked_by for jobs and job_queues that have been locked for more than 4 hours
// This corresponds to graphile-worker commit 3445867 "Periodically clear locked_at"
func ResetLockedAt(
	ctx context.Context,
	compiledSharedOptions ResetLockedSharedOptions,
	conn *pgx.Conn,
	timeProvider func() interface{},
) error {
	// Determine time source - use Node time or PostgreSQL time
	var nowExpr string
	var args []interface{}

	if compiledSharedOptions.UseNodeTime && timeProvider != nil {
		nowExpr = "$1::timestamptz"
		args = []interface{}{timeProvider()}
	} else {
		nowExpr = "now()"
		args = []interface{}{}
	}

	// Build the SQL query to clear stale locks
	query := fmt.Sprintf(`with j as (
update %s.jobs
set locked_at = null, locked_by = null
where locked_at < %s - interval '4 hours'
)
update %s.job_queues
set locked_at = null, locked_by = null
where locked_at < %s - interval '4 hours'`,
		compiledSharedOptions.EscapedWorkerSchema, nowExpr,
		compiledSharedOptions.EscapedWorkerSchema, nowExpr)

	// Execute the query
	_, err := conn.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to reset locked_at: %w", err)
	}
	return nil
}
