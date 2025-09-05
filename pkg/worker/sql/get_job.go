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

// GetJob retrieves a job from the queue using the get_job database function
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

	// Prepare query with optional 'now' parameter for useNodeTime feature
	var query string
	var args []interface{}

	if useNodeTime && timeProvider != nil {
		// Use Node's time source - pass current time as 'now' parameter
		query = fmt.Sprintf("SELECT id, queue_name, task_identifier, payload, priority, run_at, attempts, max_attempts, last_error, created_at, updated_at, key, revision, flags, locked_at, locked_by FROM %s.get_job($1, $2, '4 hours'::interval, $3::text[], $4::timestamptz)", compiledSharedOptions.EscapedWorkerSchema)
		args = []interface{}{workerId, supportedTaskNames, flagsToSkip, timeProvider()}
	} else {
		// Use PostgreSQL's time source (default behavior)
		query = fmt.Sprintf("SELECT id, queue_name, task_identifier, payload, priority, run_at, attempts, max_attempts, last_error, created_at, updated_at, key, revision, flags, locked_at, locked_by FROM %s.get_job($1, $2, '4 hours'::interval, $3::text[])", compiledSharedOptions.EscapedWorkerSchema)
		args = []interface{}{workerId, supportedTaskNames, flagsToSkip}
	}

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
