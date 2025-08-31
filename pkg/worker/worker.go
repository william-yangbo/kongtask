package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// Constants for error handling (matching graphile-worker)
const (
	DefaultPollInterval  = 2 * time.Second // Default poll interval (matches graphile-worker)
	MaxContiguousErrors  = 10              // Maximum consecutive errors before giving up (matches graphile-worker)
	ErrorRetryDelay      = 1 * time.Second // Delay before retrying after error
	FatalErrorRetryDelay = 5 * time.Second // Delay before retrying after fatal error
)

// Job represents a job from the database (v0.4.0 alignment)
type Job struct {
	ID             string          `json:"id"`         // Changed from int to string in v0.4.0
	QueueName      *string         `json:"queue_name"` // Changed to nullable in v0.4.0
	TaskIdentifier string          `json:"task_identifier"`
	Payload        json.RawMessage `json:"payload"`
	RunAt          time.Time       `json:"run_at"`
	AttemptCount   int             `json:"attempts"`
	MaxAttempts    int             `json:"max_attempts"`
	LastError      *string         `json:"last_error"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Key            *string         `json:"key"`       // New in v0.4.0: job_key support
	LockedAt       *time.Time      `json:"locked_at"` // New in v0.4.0: task locking
	LockedBy       *string         `json:"locked_by"` // New in v0.4.0: task locking
}

// TaskSpec represents job scheduling options (renamed from TaskOptions in v0.4.0)
type TaskSpec struct {
	QueueName   *string    `json:"queueName,omitempty"`   // The queue to run this task under
	RunAt       *time.Time `json:"runAt,omitempty"`       // Schedule this task to run in the future
	MaxAttempts *int       `json:"maxAttempts,omitempty"` // How many retries should this task get
	JobKey      *string    `json:"jobKey,omitempty"`      // New in v0.4.0: unique identifier for the job
}

// TaskHandler is a function that processes a job (v0.2.0 signature with helpers)
type TaskHandler func(ctx context.Context, payload json.RawMessage, helpers *Helpers) error

// Worker represents a job worker
type Worker struct {
	pool      *pgxpool.Pool
	schema    string
	handlers  map[string]TaskHandler
	workerID  string
	logger    *logger.Logger
	nudgeCh   chan struct{} // Channel for nudging the worker
	activeJob *Job          // Currently active job (for graceful shutdown)
	jobMutex  sync.RWMutex  // Protects activeJob access

	// New fields for enhanced worker functionality (v0.4.0 alignment)
	pollInterval     time.Duration // Configurable poll interval
	contiguousErrors int           // Count of consecutive errors
	active           bool          // Worker active status
	activeMutex      sync.RWMutex  // Protects active status
	releaseCh        chan struct{} // Channel for worker release signal
	timer            *time.Timer   // Timer for polling
	timerMutex       sync.Mutex    // Protects timer access
	continuous       bool          // Whether worker runs continuously or once
}

// NewWorker creates a new worker instance
func NewWorker(pool *pgxpool.Pool, schema string, opts ...WorkerOption) *Worker {
	w := &Worker{
		pool:             pool,
		schema:           schema,
		handlers:         make(map[string]TaskHandler),
		workerID:         fmt.Sprintf("worker-%d", time.Now().UnixNano()),
		logger:           logger.DefaultLogger.Scope(logger.LogScope{Label: "worker"}),
		nudgeCh:          make(chan struct{}, 1),
		pollInterval:     DefaultPollInterval,
		contiguousErrors: 0,
		active:           true,
		releaseCh:        make(chan struct{}),
		continuous:       true, // Default to continuous mode
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

// WithPollInterval sets a custom poll interval for the worker (v0.4.0 alignment)
func WithPollInterval(interval time.Duration) WorkerOption {
	return func(w *Worker) {
		w.pollInterval = interval
	}
}

// WithWorkerID sets a custom worker ID (v0.4.0 alignment)
func WithWorkerID(workerID string) WorkerOption {
	return func(w *Worker) {
		w.workerID = workerID
	}
}

// WithContinuous sets whether the worker runs continuously or once (v0.4.0 alignment)
func WithContinuous(continuous bool) WorkerOption {
	return func(w *Worker) {
		w.continuous = continuous
	}
}

// MakeNewWorker creates a new worker with the specified task list and options (v0.4.0 alignment)
// This function matches the graphile-worker makeNewWorker interface
func MakeNewWorker(
	pool *pgxpool.Pool,
	schema string,
	tasks map[string]TaskHandler,
	options *WorkerOptions,
	continuous bool,
) *Worker {
	// Set default options if not provided
	if options == nil {
		options = &WorkerOptions{}
	}

	// Configure worker options
	opts := []WorkerOption{}

	if options.Logger != nil {
		opts = append(opts, WithLogger(options.Logger))
	}

	if options.PollInterval > 0 {
		opts = append(opts, WithPollInterval(options.PollInterval))
	}

	if options.WorkerID != "" {
		opts = append(opts, WithWorkerID(options.WorkerID))
	}

	// Create worker
	w := NewWorker(pool, schema, opts...)

	// Set continuous mode (default true)
	w.continuous = options.Continuous
	if !w.continuous && !options.Continuous {
		w.continuous = true // Default to continuous mode
	}

	// Register all tasks
	for taskIdentifier, handler := range tasks {
		w.RegisterTask(taskIdentifier, handler)
	}

	// Store continuous mode (for future use)
	w.continuous = continuous

	return w
}

// WorkerOptions represents options for creating a worker (v0.4.0 alignment)
type WorkerOptions struct {
	PollInterval time.Duration  // How often to poll for jobs
	WorkerID     string         // Custom worker ID
	Logger       *logger.Logger // Custom logger
	Continuous   bool           // Whether worker runs continuously or once (default: true)
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
	job.ID = strconv.Itoa(*id) // Convert int to string for v0.4.0 compatibility
	job.QueueName = queueName  // Already a pointer
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

// CompleteJob marks a job as completed (v0.4.0: jobID is now string)
func (w *Worker) CompleteJob(ctx context.Context, jobID string) error {
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

// FailJob marks a job as failed (v0.4.0: jobID is now string)
func (w *Worker) FailJob(ctx context.Context, jobID string, message string) error {
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

// ProcessJob processes a single job (enhanced with execution timing in v0.4.0)
func (w *Worker) ProcessJob(ctx context.Context, job *Job) error {
	handler, exists := w.handlers[job.TaskIdentifier]
	if !exists {
		return fmt.Errorf("no handler registered for task: %s", job.TaskIdentifier)
	}

	// Set this as the active job for graceful shutdown tracking
	w.setActiveJob(job)
	defer w.clearActiveJob()

	// Create scoped logger for this job
	jobLogger := w.logger.Scope(logger.LogScope{
		WorkerID:       w.workerID,
		TaskIdentifier: job.TaskIdentifier,
		JobID:          &job.ID,
	})

	jobLogger.Info(fmt.Sprintf("Processing job %s: %s", job.ID, job.TaskIdentifier))

	// Start timing (v0.4.0 alignment)
	startTime := time.Now()

	// Create helpers for the task
	helpers := w.CreateHelpers(ctx, job)

	// Execute the task
	err := handler(ctx, job.Payload, helpers)

	// Calculate duration
	duration := time.Since(startTime)
	durationMs := float64(duration.Nanoseconds()) / 1e6

	if err != nil {
		jobLogger.Error(fmt.Sprintf("Job %s failed: %v (%.2fms)", job.ID, err, durationMs))
		if failErr := w.FailJob(ctx, job.ID, err.Error()); failErr != nil {
			jobLogger.Error(fmt.Sprintf("Failed to mark job %s as failed: %v", job.ID, failErr))
			return fmt.Errorf("failed to release job '%s' after failure '%s': %w", job.ID, err.Error(), failErr)
		}
		return err
	}

	jobLogger.Info(fmt.Sprintf("Job %s completed successfully (%.2fms)", job.ID, durationMs))
	if completeErr := w.CompleteJob(ctx, job.ID); completeErr != nil {
		jobLogger.Error(fmt.Sprintf("Failed to mark job %s as completed: %v", job.ID, completeErr))
		return fmt.Errorf("failed to release job '%s' after success: %w", job.ID, completeErr)
	}

	return nil
}

// Run starts the worker loop (enhanced with v0.4.0 features)
func (w *Worker) Run(ctx context.Context) error {
	workerLogger := w.logger.Scope(logger.LogScope{WorkerID: w.workerID})
	workerLogger.Info(fmt.Sprintf("Worker %s starting (continuous: %v)", w.workerID, w.continuous))

	// Reset contiguous errors at start
	w.contiguousErrors = 0

	// If not continuous mode, run once and return
	if !w.continuous {
		return w.RunOnce(ctx)
	}

	// Continuous mode - loop until stopped
	for w.isActive() {
		select {
		case <-ctx.Done():
			workerLogger.Info(fmt.Sprintf("Worker %s stopping (context cancelled)", w.workerID))
			w.Release()
			return ctx.Err()
		case <-w.releaseCh:
			workerLogger.Info(fmt.Sprintf("Worker %s stopping (released)", w.workerID))
			return nil
		case <-w.nudgeCh:
			// Nudged, try to get a job immediately
		default:
			// Regular polling
		}

		// Try to get and process a job
		err := w.doNext(ctx, workerLogger)
		if err != nil {
			w.contiguousErrors++
			workerLogger.Error(fmt.Sprintf("Failed to process job: %v (%d/%d)", err, w.contiguousErrors, MaxContiguousErrors))

			if w.contiguousErrors >= MaxContiguousErrors {
				w.Release()
				return fmt.Errorf("failed %d times in a row to acquire job; latest error: %w", w.contiguousErrors, err)
			}

			// Wait before retrying after error
			if w.isActive() {
				time.Sleep(ErrorRetryDelay)
			}
		} else {
			// Reset error count on success
			w.contiguousErrors = 0

			// If no job was available, wait for poll interval
			if w.GetActiveJob() == nil {
				if w.isActive() {
					time.Sleep(w.pollInterval)
				}
			}
		}
	}

	workerLogger.Info(fmt.Sprintf("Worker %s stopped", w.workerID))
	return nil
}

// doNext attempts to get and process a single job (v0.4.0 alignment)
func (w *Worker) doNext(ctx context.Context, workerLogger *logger.Logger) error {
	if !w.isActive() {
		return nil
	}

	// Check if we have any supported tasks
	if len(w.handlers) == 0 {
		return fmt.Errorf("no runnable tasks")
	}

	job, err := w.GetJob(ctx)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	if job == nil {
		// No jobs available
		return nil
	}

	if !w.isActive() {
		// Worker was released while getting job
		return nil
	}

	// Process the job
	if err := w.ProcessJob(ctx, job); err != nil {
		return fmt.Errorf("job processing failed: %w", err)
	}

	return nil
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

// Nudge sends a nudge signal to the worker to check for new jobs
// Returns true if nudge was sent, false if channel was already full
func (w *Worker) Nudge() bool {
	select {
	case w.nudgeCh <- struct{}{}:
		return true
	default:
		return false
	}
}

// GetActiveJob returns the currently active job (for graceful shutdown)
func (w *Worker) GetActiveJob() *Job {
	w.jobMutex.RLock()
	defer w.jobMutex.RUnlock()
	return w.activeJob
}

// setActiveJob sets the currently active job
func (w *Worker) setActiveJob(job *Job) {
	w.jobMutex.Lock()
	defer w.jobMutex.Unlock()
	w.activeJob = job
}

// clearActiveJob clears the currently active job
func (w *Worker) clearActiveJob() {
	w.jobMutex.Lock()
	defer w.jobMutex.Unlock()
	w.activeJob = nil
}

// isActive returns whether the worker is active (v0.4.0 alignment)
func (w *Worker) isActive() bool {
	w.activeMutex.RLock()
	defer w.activeMutex.RUnlock()
	return w.active
}

// setActive sets the worker active status (v0.4.0 alignment)
func (w *Worker) setActive(active bool) {
	w.activeMutex.Lock()
	defer w.activeMutex.Unlock()
	w.active = active
}

// GetWorkerID returns the worker ID (public method)
func (w *Worker) GetWorkerID() string {
	return w.workerID
}

// IsActive returns whether the worker is active (public method)
func (w *Worker) IsActive() bool {
	return w.isActive()
}

// Release gracefully shuts down the worker (v0.4.0 alignment)
func (w *Worker) Release() {
	if !w.isActive() {
		return
	}

	w.setActive(false)

	// Cancel any running timer
	w.cancelTimer()

	// Signal release
	select {
	case w.releaseCh <- struct{}{}:
	default:
		// Channel already has a signal or is closed
	}
}

// cancelTimer safely cancels the worker timer (v0.4.0 alignment)
func (w *Worker) cancelTimer() {
	w.timerMutex.Lock()
	defer w.timerMutex.Unlock()

	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
}
