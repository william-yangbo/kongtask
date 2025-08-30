package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Job represents a job from the database
type Job struct {
	ID             int             `json:"id"`
	QueueName      string          `json:"queue_name"`
	TaskIdentifier string          `json:"task_identifier"`
	Payload        json.RawMessage `json:"payload"`
	Priority       int             `json:"priority"`
	RunAt          time.Time       `json:"run_at"`
	AttemptCount   int             `json:"attempts"`
	MaxAttempts    int             `json:"max_attempts"`
	LastError      *string         `json:"last_error"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// TaskHandler is a function that processes a job
type TaskHandler func(ctx context.Context, job *Job) error

// Worker represents a job worker
type Worker struct {
	pool     *pgxpool.Pool
	schema   string
	handlers map[string]TaskHandler
	workerID string
}

// NewWorker creates a new worker instance
func NewWorker(pool *pgxpool.Pool, schema string) *Worker {
	return &Worker{
		pool:     pool,
		schema:   schema,
		handlers: make(map[string]TaskHandler),
		workerID: fmt.Sprintf("worker-%d", time.Now().UnixNano()),
	}
}

// RegisterTask registers a task handler
func (w *Worker) RegisterTask(taskIdentifier string, handler TaskHandler) {
	w.handlers[taskIdentifier] = handler
}

// AddJob adds a job to the queue (corresponds to graphile-worker add_job)
func (w *Worker) AddJob(ctx context.Context, taskIdentifier string, payload interface{}) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	query := fmt.Sprintf("SELECT %s.add_job($1, $2)", w.schema)
	_, err = conn.Exec(ctx, query, taskIdentifier, string(payloadJSON))
	if err != nil {
		return fmt.Errorf("failed to add job: %w", err)
	}

	return nil
}

// GetJob gets a job from the queue (corresponds to graphile-worker get_job)
func (w *Worker) GetJob(ctx context.Context) (*Job, error) {
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	query := fmt.Sprintf("SELECT * FROM %s.get_job($1)", w.schema)
	row := conn.QueryRow(ctx, query, w.workerID)

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
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // No jobs available
		}
		return nil, fmt.Errorf("failed to scan job: %w", err)
	}

	// Check if the result is NULL (no job available)
	if id == nil {
		return nil, nil
	}

	// Populate the job struct
	job.ID = *id
	job.QueueName = *queueName
	job.TaskIdentifier = *taskIdentifier
	job.Payload = *payload
	job.Priority = *priority
	job.RunAt = *runAt
	job.AttemptCount = *attemptCount
	job.MaxAttempts = *maxAttempts
	job.LastError = lastError
	job.CreatedAt = *createdAt
	job.UpdatedAt = *updatedAt

	return &job, nil
}

// CompleteJob marks a job as completed
func (w *Worker) CompleteJob(ctx context.Context, jobID int) error {
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	query := fmt.Sprintf("SELECT %s.complete_job($1, $2)", w.schema)
	_, err = conn.Exec(ctx, query, w.workerID, jobID)
	if err != nil {
		return fmt.Errorf("failed to complete job: %w", err)
	}

	return nil
}

// FailJob marks a job as failed
func (w *Worker) FailJob(ctx context.Context, jobID int, message string) error {
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	query := fmt.Sprintf("SELECT %s.fail_job($1, $2, $3)", w.schema)
	_, err = conn.Exec(ctx, query, w.workerID, jobID, message)
	if err != nil {
		return fmt.Errorf("failed to fail job: %w", err)
	}

	return nil
}

// ProcessJob processes a single job
func (w *Worker) ProcessJob(ctx context.Context, job *Job) error {
	handler, exists := w.handlers[job.TaskIdentifier]
	if !exists {
		return fmt.Errorf("no handler registered for task: %s", job.TaskIdentifier)
	}

	log.Printf("Processing job %d: %s", job.ID, job.TaskIdentifier)

	err := handler(ctx, job)
	if err != nil {
		log.Printf("Job %d failed: %v", job.ID, err)
		if failErr := w.FailJob(ctx, job.ID, err.Error()); failErr != nil {
			log.Printf("Failed to mark job %d as failed: %v", job.ID, failErr)
		}
		return err
	}

	log.Printf("Job %d completed successfully", job.ID)
	if completeErr := w.CompleteJob(ctx, job.ID); completeErr != nil {
		log.Printf("Failed to mark job %d as completed: %v", job.ID, completeErr)
		return completeErr
	}

	return nil
}

// Run starts the worker loop
func (w *Worker) Run(ctx context.Context) error {
	log.Printf("Worker %s starting", w.workerID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %s stopping", w.workerID)
			return ctx.Err()
		default:
			job, err := w.GetJob(ctx)
			if err != nil {
				log.Printf("Failed to get job: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}

			if job == nil {
				// No jobs available, wait a bit
				time.Sleep(1 * time.Second)
				continue
			}

			if err := w.ProcessJob(ctx, job); err != nil {
				log.Printf("Job processing failed: %v", err)
			}
		}
	}
}
