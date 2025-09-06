package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/events"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// Constants for error handling (matching graphile-worker)
const (
	DefaultPollInterval  = 2 * time.Second // Default poll interval (matches graphile-worker)
	MaxContiguousErrors  = 10              // Maximum consecutive errors before giving up (matches graphile-worker)
	ErrorRetryDelay      = 1 * time.Second // Delay before retrying after error
	FatalErrorRetryDelay = 5 * time.Second // Delay before retrying after fatal error
)

// JobKeyMode constants for controlling job key behavior (feature from commit e7ab91e)
const (
	JobKeyModeReplace       = "replace"         // (default) overwrites unlocked job with new values, primarily for debouncing
	JobKeyModePreserveRunAt = "preserve_run_at" // overwrites unlocked job but preserves run_at, primarily for throttling
	JobKeyModeUnsafeDedupe  = "unsafe_dedupe"   // dangerous: won't update existing job even if locked/failed
)

// generateWorkerID generates a cryptographically secure worker ID
// Inspired by graphile-worker v0.5.0 improvement (commit 69938b4)
func generateWorkerID() string {
	// Generate 9 random bytes (same as graphile-worker v0.5.0)
	randomBytes := make([]byte, 9)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("worker-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("worker-%s", hex.EncodeToString(randomBytes))
}

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

// ForbiddenFlagsFunc is a function that returns forbidden flags
type ForbiddenFlagsFunc func() ([]string, error)

// WithPgClientFunc provides database connection handling
type WithPgClientFunc func(func(*pgx.Conn) error) error

// TaskSpec represents job scheduling options (renamed from TaskOptions in v0.4.0)
type TaskSpec struct {
	QueueName   *string    `json:"queueName,omitempty"`   // The queue to run this task under (only specify if you want jobs in this queue to run serially). (Default: null)
	RunAt       *time.Time `json:"runAt,omitempty"`       // A Date to schedule this task to run in the future. (Default: now)
	Priority    *int       `json:"priority,omitempty"`    // Jobs are executed in numerically ascending order of priority (jobs with a numerically smaller priority are run first). (Default: 0)
	MaxAttempts *int       `json:"maxAttempts,omitempty"` // How many retries should this task get? (Default: 25)
	JobKey      *string    `json:"jobKey,omitempty"`      // Unique identifier for the job, can be used to replace, update or remove it later if needed. (Default: null)
	// Modifies the behavior of `jobKey`; when 'replace' all attributes will be updated, when 'preserve_run_at' all attributes except 'run_at' will be updated,
	// when 'unsafe_dedupe' a new job will only be added if no existing job (including locked jobs and permanently failed jobs) with matching job key exists. (Default: 'replace')
	JobKeyMode *string  `json:"jobKeyMode,omitempty"`
	Flags      []string `json:"flags,omitempty"` // Flags for the job, can be used to dynamically filter which jobs can and cannot run at runtime. (Default: null)
}

// RescheduleOptions represents options for rescheduling jobs (commit 27dee4d)
type RescheduleOptions struct {
	RunAt       *time.Time `json:"runAt,omitempty"`       // New run time for the jobs
	Priority    *int       `json:"priority,omitempty"`    // New priority for the jobs
	Attempts    *int       `json:"attempts,omitempty"`    // Reset attempt count
	MaxAttempts *int       `json:"maxAttempts,omitempty"` // New maximum attempts
}

// TaskHandler is a function that processes a job (v0.2.0 signature with helpers)
type TaskHandler func(ctx context.Context, payload json.RawMessage, helpers *Helpers) error

// ForbiddenFlagsFn is a function that returns forbidden flags dynamically (commit fb9b249)
type ForbiddenFlagsFn func() ([]string, error)

// Worker represents a job worker
type Worker struct {
	pool      *pgxpool.Pool
	schema    string
	handlers  map[string]TaskHandler
	workerID  string
	logger    *logger.Logger
	eventBus  *events.EventBus // Event bus for emitting worker events
	nudgeCh   chan struct{}    // Channel for nudging the worker
	activeJob *Job             // Currently active job (for graceful shutdown)
	jobMutex  sync.RWMutex     // Protects activeJob access

	// New fields for enhanced worker functionality (v0.4.0 alignment)
	pollInterval         time.Duration    // Configurable poll interval
	contiguousErrors     int              // Count of consecutive errors
	active               bool             // Worker active status
	activeMutex          sync.RWMutex     // Protects active status
	releaseCh            chan struct{}    // Channel for worker release signal
	timer                *time.Timer      // Timer for polling
	timerMutex           sync.Mutex       // Protects timer access
	continuous           bool             // Whether worker runs continuously or once
	noPreparedStatements bool             // Disable prepared statements for pgBouncer compatibility
	forbiddenFlags       []string         // Static forbidden flags (commit fb9b249)
	forbiddenFlagsFn     ForbiddenFlagsFn // Dynamic forbidden flags function (commit fb9b249)
	useNodeTime          bool             // Use Node's time source instead of PostgreSQL's (commit 5a09a37)
	timeProvider         TimeProvider     // Time provider for useNodeTime feature (testing support)
}

// NewWorker creates a new worker instance
func NewWorker(pool *pgxpool.Pool, schema string, opts ...WorkerOption) *Worker {
	w := &Worker{
		pool:             pool,
		schema:           schema,
		handlers:         make(map[string]TaskHandler),
		workerID:         generateWorkerID(), // Use secure worker ID generation (v0.5.0 improvement)
		logger:           logger.DefaultLogger.Scope(logger.LogScope{Label: "worker"}),
		nudgeCh:          make(chan struct{}, 1),
		pollInterval:     DefaultPollInterval,
		contiguousErrors: 0,
		active:           true,
		releaseCh:        make(chan struct{}),
		continuous:       true,                  // Default to continuous mode
		timeProvider:     NewRealTimeProvider(), // Default to real time
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

// WithNoPreparedStatements disables prepared statements for pgBouncer compatibility
func WithNoPreparedStatements(noPreparedStatements bool) WorkerOption {
	return func(w *Worker) {
		w.noPreparedStatements = noPreparedStatements
	}
}

// WithForbiddenFlags sets static forbidden flags (commit fb9b249)
func WithForbiddenFlags(flags []string) WorkerOption {
	return func(w *Worker) {
		w.forbiddenFlags = flags
	}
}

// WithForbiddenFlagsFn sets dynamic forbidden flags function (commit fb9b249)
func WithForbiddenFlagsFn(fn ForbiddenFlagsFn) WorkerOption {
	return func(w *Worker) {
		w.forbiddenFlagsFn = fn
	}
}

// WithEventBus sets the event bus for the worker
func WithEventBus(eventBus *events.EventBus) WorkerOption {
	return func(w *Worker) {
		w.eventBus = eventBus
	}
}

// WithUseNodeTime sets whether to use Node's time source instead of PostgreSQL's (commit 5a09a37)
func WithUseNodeTime(useNodeTime bool) WorkerOption {
	return func(w *Worker) {
		w.useNodeTime = useNodeTime
	}
}

// WithTimeProvider sets a custom time provider for testing
func WithTimeProvider(timeProvider TimeProvider) WorkerOption {
	return func(w *Worker) {
		w.timeProvider = timeProvider
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

	if options.NoPreparedStatements {
		opts = append(opts, WithNoPreparedStatements(options.NoPreparedStatements))
	}

	if options.ForbiddenFlags != nil {
		opts = append(opts, WithForbiddenFlags(options.ForbiddenFlags))
	}

	if options.ForbiddenFlagsFn != nil {
		opts = append(opts, WithForbiddenFlagsFn(options.ForbiddenFlagsFn))
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
	PollInterval         time.Duration    // How often to poll for jobs
	WorkerID             string           // Custom worker ID
	Logger               *logger.Logger   // Custom logger
	Continuous           bool             // Whether worker runs continuously or once (default: true)
	NoPreparedStatements bool             // Disable prepared statements for pgBouncer compatibility
	UseNodeTime          bool             // Use Node's time source instead of PostgreSQL's (commit 5a09a37)
	ForbiddenFlags       []string         // Static forbidden flags (commit fb9b249)
	ForbiddenFlagsFn     ForbiddenFlagsFn // Dynamic forbidden flags function (commit fb9b249)
}

// RegisterTask registers a task handler
func (w *Worker) RegisterTask(taskIdentifier string, handler TaskHandler) {
	isFirstTask := len(w.handlers) == 0
	w.handlers[taskIdentifier] = handler

	// Emit worker:create event when first task is registered
	if isFirstTask && w.eventBus != nil {
		// Create task list for the event
		tasks := make(map[string]interface{})
		for taskName := range w.handlers {
			tasks[taskName] = true // Just indicate presence of task
		}

		w.eventBus.Emit(events.WorkerCreate, map[string]interface{}{
			"worker": map[string]interface{}{
				"workerId": w.workerID,
			},
			"tasks": tasks,
		})
	}
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
	// Emit worker:getJob:start event (commit 92f4b3d alignment)
	if w.eventBus != nil {
		w.eventBus.Emit(events.WorkerGetJobStart, map[string]interface{}{
			"workerId": w.workerID,
		})
	}

	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	// Determine forbidden flags
	var forbiddenFlags []string
	if w.forbiddenFlags != nil {
		forbiddenFlags = w.forbiddenFlags
	} else if w.forbiddenFlagsFn != nil {
		flags, err := w.forbiddenFlagsFn()
		if err != nil {
			return nil, fmt.Errorf("failed to get forbidden flags: %w", err)
		}
		forbiddenFlags = flags
	}

	// Prepare query with optional 'now' parameter for useNodeTime feature (commit 5a09a37)
	var query string
	var args []interface{}

	if w.useNodeTime {
		// Use Node's time source - pass current time as 'now' parameter
		query = fmt.Sprintf("SELECT id, queue_name, task_identifier, payload, priority, run_at, attempts, max_attempts, last_error, created_at, updated_at, key, revision, flags, locked_at, locked_by FROM %s.get_job($1, null, '4 hours'::interval, $2, $3)", w.schema)
		args = []interface{}{w.workerID, forbiddenFlags, w.timeProvider.Now()}
	} else {
		// Use PostgreSQL's time source (default behavior)
		query = fmt.Sprintf("SELECT id, queue_name, task_identifier, payload, priority, run_at, attempts, max_attempts, last_error, created_at, updated_at, key, revision, flags, locked_at, locked_by FROM %s.get_job($1, null, '4 hours'::interval, $2)", w.schema)
		args = []interface{}{w.workerID, forbiddenFlags}
	}

	var row pgx.Row
	if w.noPreparedStatements {
		// Use simple protocol to avoid prepared statements (for pgBouncer compatibility)
		// Note: QueryExecModeSimpleProtocol must be passed separately from args
		if w.useNodeTime {
			row = conn.QueryRow(ctx, query, pgx.QueryExecModeSimpleProtocol, w.workerID, forbiddenFlags, w.timeProvider.Now())
		} else {
			row = conn.QueryRow(ctx, query, pgx.QueryExecModeSimpleProtocol, w.workerID, forbiddenFlags)
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
			// Emit worker:getJob:empty event
			if w.eventBus != nil {
				w.eventBus.Emit(events.WorkerGetJobEmpty, map[string]interface{}{
					"worker": map[string]interface{}{
						"workerId": w.workerID,
					},
				})
			}
			return nil, nil // No jobs available
		}
		// Emit worker:getJob:error event
		if w.eventBus != nil {
			w.eventBus.Emit(events.WorkerGetJobError, map[string]interface{}{
				"worker": map[string]interface{}{
					"workerId": w.workerID,
				},
				"error": err.Error(),
			})
		}
		return nil, fmt.Errorf("failed to scan job: %w", err)
	}

	// Check if the result is NULL (no job available)
	if id == nil || taskIdentifier == nil || payload == nil || priority == nil || runAt == nil ||
		attemptCount == nil || maxAttempts == nil || createdAt == nil || updatedAt == nil {
		// Emit worker:getJob:empty event
		if w.eventBus != nil {
			w.eventBus.Emit(events.WorkerGetJobEmpty, map[string]interface{}{
				"worker": map[string]interface{}{
					"workerId": w.workerID,
				},
			})
		}
		return nil, nil
	}

	// Populate the job struct
	job.ID = strconv.Itoa(*id) // Convert int to string for v0.4.0 compatibility
	job.QueueName = queueName  // Already a pointer
	job.TaskIdentifier = *taskIdentifier
	job.Payload = *payload
	job.Priority = *priority
	job.RunAt = *runAt
	job.AttemptCount = *attemptCount
	job.MaxAttempts = *maxAttempts
	job.LastError = lastError
	job.CreatedAt = *createdAt
	job.UpdatedAt = *updatedAt
	job.Key = key
	if revision != nil {
		job.Revision = *revision
	}

	// Parse flags JSON if present
	if flags != nil {
		var flagsMap map[string]bool
		if err := json.Unmarshal(*flags, &flagsMap); err != nil {
			return nil, fmt.Errorf("failed to parse flags: %w", err)
		}
		// Convert to map[string]interface{}
		job.Flags = make(map[string]interface{})
		for k, v := range flagsMap {
			job.Flags[k] = v
		}
	} else {
		job.Flags = nil
	}

	job.LockedAt = lockedAt
	job.LockedBy = lockedBy

	return &job, nil
}

// CompleteJob marks a job as completed (v0.4.0: jobID is now string, moved from database function)
func (w *Worker) CompleteJob(ctx context.Context, jobID string) error {
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	query := fmt.Sprintf(`with j as (
delete from %s.jobs
where id = $2
returning *
)
update %s.job_queues
set locked_by = null, locked_at = null
from j
where job_queues.queue_name = j.queue_name and job_queues.locked_by = $1;`, w.schema, w.schema)

	if w.noPreparedStatements {
		// Use simple protocol to avoid prepared statements (for pgBouncer compatibility)
		_, err = conn.Exec(ctx, query, pgx.QueryExecModeSimpleProtocol, w.workerID, jobID)
	} else {
		_, err = conn.Exec(ctx, query, w.workerID, jobID)
	}
	if err != nil {
		return fmt.Errorf("failed to complete job: %w", err)
	}

	return nil
}

// FailJob marks a job as failed (v0.4.0: jobID is now string, moved from database function)
func (w *Worker) FailJob(ctx context.Context, jobID string, message string) error {
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

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
where job_queues.queue_name = j.queue_name and job_queues.locked_by = $1;`, w.schema, w.schema)

	if w.noPreparedStatements {
		// Use simple protocol to avoid prepared statements (for pgBouncer compatibility)
		_, err = conn.Exec(ctx, query, pgx.QueryExecModeSimpleProtocol, w.workerID, jobID, message)
	} else {
		_, err = conn.Exec(ctx, query, w.workerID, jobID, message)
	}
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

	// Emit job:start event
	if w.eventBus != nil {
		w.eventBus.Emit(events.JobStart, map[string]interface{}{
			"worker": map[string]interface{}{
				"workerId": w.workerID,
			},
			"job": map[string]interface{}{
				"id":             job.ID,
				"taskIdentifier": job.TaskIdentifier,
				"attempts":       job.AttemptCount,
				"max_attempts":   job.MaxAttempts,
				"queue_name":     job.QueueName,
				"priority":       job.Priority,
				"run_at":         job.RunAt,
				"created_at":     job.CreatedAt,
			},
		})
	}

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
		// Emit job:error event
		if w.eventBus != nil {
			w.eventBus.Emit(events.JobError, map[string]interface{}{
				"worker": map[string]interface{}{
					"workerId": w.workerID,
				},
				"job": map[string]interface{}{
					"id":             job.ID,
					"taskIdentifier": job.TaskIdentifier,
					"attempts":       job.AttemptCount,
					"max_attempts":   job.MaxAttempts,
				},
				"error":       err.Error(),
				"duration_ms": durationMs,
			})

			// Emit job:failed event if this was the final attempt
			if job.AttemptCount >= job.MaxAttempts {
				w.eventBus.Emit(events.JobFailed, map[string]interface{}{
					"worker": map[string]interface{}{
						"workerId": w.workerID,
					},
					"job": map[string]interface{}{
						"id":             job.ID,
						"taskIdentifier": job.TaskIdentifier,
						"attempts":       job.AttemptCount,
						"max_attempts":   job.MaxAttempts,
					},
					"error":       err.Error(),
					"duration_ms": durationMs,
				})
			}
		}

		jobLogger.Error(fmt.Sprintf("Job %s failed: %v (%.2fms)\nattempt (%d of %d)", job.ID, err, durationMs, job.AttemptCount, job.MaxAttempts))
		if failErr := w.FailJob(ctx, job.ID, err.Error()); failErr != nil {
			jobLogger.Error(fmt.Sprintf("Failed to mark job %s as failed: %v", job.ID, failErr))
			return fmt.Errorf("failed to release job '%s' after failure '%s': %w", job.ID, err.Error(), failErr)
		}

		// Emit job:complete event after job result is written to database (commit 92f4b3d alignment)
		if w.eventBus != nil {
			w.eventBus.Emit(events.JobComplete, map[string]interface{}{
				"worker": map[string]interface{}{
					"workerId": w.workerID,
				},
				"job": map[string]interface{}{
					"id":             job.ID,
					"taskIdentifier": job.TaskIdentifier,
					"attempts":       job.AttemptCount,
					"max_attempts":   job.MaxAttempts,
				},
				"error": err.Error(), // Job failed with error (commit 63c2ee6 alignment)
			})
		}

		return err
	}

	// Emit job:success event
	if w.eventBus != nil {
		w.eventBus.Emit(events.JobSuccess, map[string]interface{}{
			"worker": map[string]interface{}{
				"workerId": w.workerID,
			},
			"job": map[string]interface{}{
				"id":             job.ID,
				"taskIdentifier": job.TaskIdentifier,
				"attempts":       job.AttemptCount,
				"max_attempts":   job.MaxAttempts,
			},
			"duration_ms": durationMs,
		})
	}

	jobLogger.Info(fmt.Sprintf("Job %s completed successfully (%.2fms)", job.ID, durationMs))
	if completeErr := w.CompleteJob(ctx, job.ID); completeErr != nil {
		jobLogger.Error(fmt.Sprintf("Failed to mark job %s as completed: %v", job.ID, completeErr))
		return fmt.Errorf("failed to release job '%s' after success: %w", job.ID, completeErr)
	}

	// Emit job:complete event after job result is written to database (commit 92f4b3d alignment)
	if w.eventBus != nil {
		w.eventBus.Emit(events.JobComplete, map[string]interface{}{
			"worker": map[string]interface{}{
				"workerId": w.workerID,
			},
			"job": map[string]interface{}{
				"id":             job.ID,
				"taskIdentifier": job.TaskIdentifier,
				"attempts":       job.AttemptCount,
				"max_attempts":   job.MaxAttempts,
			},
			"error": nil, // Job completed successfully (commit 63c2ee6 alignment)
		})
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
				// Emit worker:fatalError event
				if w.eventBus != nil {
					w.eventBus.Emit(events.WorkerFatalError, map[string]interface{}{
						"worker": map[string]interface{}{
							"workerId": w.workerID,
						},
						"error":    err.Error(),
						"jobError": nil, // No specific job error in this case
					})
				}

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

	// Emit worker:release event
	if w.eventBus != nil {
		w.eventBus.Emit(events.WorkerRelease, map[string]interface{}{
			"worker": map[string]interface{}{
				"workerId": w.workerID,
			},
		})
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

	// Emit worker:stop event
	if w.eventBus != nil {
		w.eventBus.Emit(events.WorkerStop, map[string]interface{}{
			"worker": map[string]interface{}{
				"workerId": w.workerID,
			},
		})
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
