package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Job represents a job instance
type Job struct {
	ID             string                 `json:"id"`
	QueueName      *string                `json:"queue_name"`
	TaskIdentifier string                 `json:"task_identifier"`
	Payload        json.RawMessage        `json:"payload"`
	Priority       int                    `json:"priority"`
	RunAt          time.Time              `json:"run_at"`
	AttemptCount   int                    `json:"attempts"`
	MaxAttempts    int                    `json:"max_attempts"`
	LastError      *string                `json:"last_error"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	Key            *string                `json:"key"`
	Revision       int                    `json:"revision"`
	LockedAt       *time.Time             `json:"locked_at"`
	LockedBy       *string                `json:"locked_by"`
	Flags          map[string]interface{} `json:"flags"`
}

// CompiledSharedOptions represents shared configuration options
type CompiledSharedOptions struct {
	Schema               string
	EscapedWorkerSchema  string
	WorkerSchema         string
	NoPreparedStatements bool
}

// GetJob retrieves a job from the queue using inline SQL (moved from database function)
func GetJob(
	ctx context.Context,
	compiledSharedOptions CompiledSharedOptions,
	pool *pgxpool.Pool,
	workerId string,
	supportedTaskNames []string,
	useNodeTime bool,
	flagsToSkip []string,
	timeProvider func() time.Time,
) (*Job, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	// Determine time source - use Node time or PostgreSQL time
	var nowExpr string
	var args []interface{}

	if useNodeTime && timeProvider != nil {
		nowExpr = "$4::timestamptz"
		args = []interface{}{workerId, supportedTaskNames, flagsToSkip, timeProvider()}
	} else {
		nowExpr = "now()"
		args = []interface{}{workerId, supportedTaskNames, flagsToSkip}
	}

	// Build the complex inline SQL matching graphile-worker's implementation
	query := fmt.Sprintf(`with j as (
  select jobs.queue_name, jobs.id
    from %s.jobs
    where (jobs.locked_at is null or jobs.locked_at < (%s - interval '4 hours'))
    and (
      jobs.queue_name is null
    or
      exists (
        select 1
        from %s.job_queues
        where job_queues.queue_name = jobs.queue_name
        and (job_queues.locked_at is null or job_queues.locked_at < (%s - interval '4 hours'))
        for update
        skip locked
      )
    )
    and run_at <= %s
    and attempts < max_attempts
    and ($2::text[] is null or task_identifier = any($2::text[]))
    and ($3::text[] is null or (flags ?| $3::text[]) is not true)
    order by priority asc, run_at asc, id asc
    limit 1
    for update
    skip locked
),
q as (
  update %s.job_queues
    set
      locked_by = $1::text,
      locked_at = %s
    from j
    where job_queues.queue_name = j.queue_name
)
  update %s.jobs
    set
      attempts = jobs.attempts + 1,
      locked_by = $1::text,
      locked_at = %s
    from j
    where jobs.id = j.id
    returning jobs.id, jobs.queue_name, jobs.task_identifier, jobs.payload, jobs.priority, jobs.run_at, jobs.attempts, jobs.max_attempts, jobs.last_error, jobs.created_at, jobs.updated_at, jobs.key, jobs.revision, jobs.flags, jobs.locked_at, jobs.locked_by`,
		compiledSharedOptions.EscapedWorkerSchema, nowExpr,
		compiledSharedOptions.EscapedWorkerSchema, nowExpr,
		nowExpr,
		compiledSharedOptions.EscapedWorkerSchema, nowExpr,
		compiledSharedOptions.EscapedWorkerSchema, nowExpr)

	var row pgx.Row
	if compiledSharedOptions.NoPreparedStatements {
		// Use simple protocol to avoid prepared statements (for pgBouncer compatibility)
		if useNodeTime && timeProvider != nil {
			row = conn.QueryRow(ctx, query, pgx.QueryExecModeSimpleProtocol, workerId, supportedTaskNames, flagsToSkip, timeProvider())
		} else {
			row = conn.QueryRow(ctx, query, pgx.QueryExecModeSimpleProtocol, workerId, supportedTaskNames, flagsToSkip)
		}
	} else {
		row = conn.QueryRow(ctx, query, args...)
	}

	var job Job
	var id *int
	var queueName *string
	var taskIdentifier *string
	var payload *json.RawMessage
	var priority *int
	var runAt *time.Time
	var attemptCount *int
	var maxAttempts *int
	var lastError *string
	var createdAt *time.Time
	var updatedAt *time.Time
	var key *string
	var revision *int
	var flags *json.RawMessage
	var lockedAt *time.Time
	var lockedBy *string

	err = row.Scan(
		&id,
		&queueName,
		&taskIdentifier,
		&payload,
		&priority,
		&runAt,
		&attemptCount,
		&maxAttempts,
		&lastError,
		&createdAt,
		&updatedAt,
		&key,
		&revision,
		&flags,
		&lockedAt,
		&lockedBy,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // No jobs available
		}
		return nil, fmt.Errorf("failed to scan job: %w", err)
	}

	// Handle null values and convert types
	if id != nil {
		job.ID = fmt.Sprintf("%d", *id)
	}
	if queueName != nil {
		job.QueueName = queueName
	}
	if taskIdentifier != nil {
		job.TaskIdentifier = *taskIdentifier
	}
	if payload != nil {
		job.Payload = *payload
	}
	if priority != nil {
		job.Priority = *priority
	}
	if runAt != nil {
		job.RunAt = *runAt
	}
	if attemptCount != nil {
		job.AttemptCount = *attemptCount
	}
	if maxAttempts != nil {
		job.MaxAttempts = *maxAttempts
	}
	if lastError != nil {
		job.LastError = lastError
	}
	if createdAt != nil {
		job.CreatedAt = *createdAt
	}
	if updatedAt != nil {
		job.UpdatedAt = *updatedAt
	}
	if key != nil {
		job.Key = key
	}
	if revision != nil {
		job.Revision = *revision
	}
	if flags != nil {
		var flagsMap map[string]bool
		if err := json.Unmarshal(*flags, &flagsMap); err == nil {
			// Convert to map[string]interface{}
			job.Flags = make(map[string]interface{})
			for k, v := range flagsMap {
				job.Flags[k] = v
			}
		}
	} else {
		job.Flags = nil
	}
	if lockedAt != nil {
		job.LockedAt = lockedAt
	}
	if lockedBy != nil {
		job.LockedBy = lockedBy
	}

	return &job, nil
}
