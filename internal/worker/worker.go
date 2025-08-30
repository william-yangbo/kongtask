package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/internal/logger"
)

// Job represents a job from the database
type Job struct {
	ID             int             `json:"id"`
	QueueName      string          `json:"queue_name"`
	TaskIdentifier string          `json:"task_identifier"`
	Payload        json.RawMessage `json:"payload"`
	RunAt          time.Time       `json:"run_at"`
	AttemptCount   int             `json:"attempts"`
	MaxAttempts    int             `json:"max_attempts"`
	LastError      *string         `json:"last_error"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// TaskHandler is a function that processes a job (v0.2.0 signature with helpers)
type TaskHandler func(ctx context.Context, payload json.RawMessage, helpers *Helpers) error

// Worker represents a job worker
type Worker struct {
	pool     *pgxpool.Pool
	schema   string
	handlers map[string]TaskHandler
	workerID string
	logger   *logger.Logger
}

// NewWorker creates a new worker instance
func NewWorker(pool *pgxpool.Pool, schema string, opts ...WorkerOption) *Worker {
	w := &Worker{
		pool:     pool,
		schema:   schema,
		handlers: make(map[string]TaskHandler),
		workerID: fmt.Sprintf("worker-%d", time.Now().UnixNano()),
		logger:   logger.DefaultLogger.Scope(logger.LogScope{Label: "worker"}),
	}

	// Apply options
	for _, opt := range opts {
		opt(w)
	}

	return w
}

// WorkerOption is a function that configures a Worker
type WorkerOption func(*Worker)

// WithLogger sets a custom logger for the worker
func WithLogger(l *logger.Logger) WorkerOption {
	return func(w *Worker) {
		w.logger = l.Scope(logger.LogScope{Label: "worker"})
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

	query := fmt.Sprintf("SELECT id, queue_name, task_identifier, payload, run_at, attempts, max_attempts, last_error, created_at, updated_at FROM %s.get_job($1)", w.schema)
	row := conn.QueryRow(ctx, query, w.workerID)

	var job Job
	var id *int
	var queueName *string
	var taskIdentifier *string
	var payload *json.RawMessage
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

	// Create scoped logger for this job
	jobLogger := w.logger.Scope(logger.LogScope{
		WorkerID:       w.workerID,
		TaskIdentifier: job.TaskIdentifier,
		JobID:          &job.ID,
	})

	jobLogger.Info(fmt.Sprintf("Processing job %d: %s", job.ID, job.TaskIdentifier))

	// Create helpers for the task
	helpers := w.CreateHelpers(ctx, job)

	err := handler(ctx, job.Payload, helpers)
	if err != nil {
		jobLogger.Error(fmt.Sprintf("Job %d failed: %v", job.ID, err))
		if failErr := w.FailJob(ctx, job.ID, err.Error()); failErr != nil {
			jobLogger.Error(fmt.Sprintf("Failed to mark job %d as failed: %v", job.ID, failErr))
		}
		return err
	}

	jobLogger.Info(fmt.Sprintf("Job %d completed successfully", job.ID))
	if completeErr := w.CompleteJob(ctx, job.ID); completeErr != nil {
		jobLogger.Error(fmt.Sprintf("Failed to mark job %d as completed: %v", job.ID, completeErr))
		return completeErr
	}

	return nil
}

// Run starts the worker loop
func (w *Worker) Run(ctx context.Context) error {
	workerLogger := w.logger.Scope(logger.LogScope{WorkerID: w.workerID})
	workerLogger.Info(fmt.Sprintf("Worker %s starting", w.workerID))

	for {
		select {
		case <-ctx.Done():
			workerLogger.Info(fmt.Sprintf("Worker %s stopping", w.workerID))
			return ctx.Err()
		default:
			job, err := w.GetJob(ctx)
			if err != nil {
				workerLogger.Error(fmt.Sprintf("Failed to get job: %v", err))
				time.Sleep(1 * time.Second)
				continue
			}

			if job == nil {
				// No jobs available, wait a bit
				time.Sleep(1 * time.Second)
				continue
			}

			if err := w.ProcessJob(ctx, job); err != nil {
				workerLogger.Error(fmt.Sprintf("Job processing failed: %v", err))
			}
		}
	}
}

// RunOnce processes all available jobs then exits (v0.2.0 feature)
func (w *Worker) RunOnce(ctx context.Context) error {
	workerLogger := w.logger.Scope(logger.LogScope{WorkerID: w.workerID})
	workerLogger.Info(fmt.Sprintf("Worker %s starting (once mode)", w.workerID))

	processedJobs := 0
	for {
		select {
		case <-ctx.Done():
			workerLogger.Info(fmt.Sprintf("Worker %s stopping (context cancelled)", w.workerID))
			return ctx.Err()
		default:
			job, err := w.GetJob(ctx)
			if err != nil {
				workerLogger.Error(fmt.Sprintf("Failed to get job: %v", err))
				return err
			}

			if job == nil {
				// No jobs available, exit
				workerLogger.Info(fmt.Sprintf("Worker %s finished - no more jobs available. Processed %d jobs", w.workerID, processedJobs))
				return nil
			}

			if err := w.ProcessJob(ctx, job); err != nil {
				workerLogger.Error(fmt.Sprintf("Job processing failed: %v", err))
				// Continue processing other jobs even if one fails
			} else {
				processedJobs++
			}
		}
	}
}
