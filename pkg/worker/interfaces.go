package worker

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// WithPgClient represents a function that provides access to a PostgreSQL client
// This mirrors the TypeScript WithPgClient type
type WithPgClient func(ctx context.Context, callback func(tx pgx.Tx) error) error

// AddJobFunction represents the addJob interface implemented throughout the library
// This mirrors the TypeScript AddJobFunction interface
type AddJobFunction func(
	ctx context.Context,
	identifier string,
	payload interface{},
	spec ...TaskSpec,
) error

// Task represents a task function that processes a job payload
// This mirrors the TypeScript Task type
type Task func(payload json.RawMessage, helpers *JobHelpers) error

// TaskList represents a collection of named tasks
// This mirrors the TypeScript TaskList interface
type TaskList map[string]TaskHandler

// WatchedTaskList represents a task list that can be updated when tasks change
// This mirrors the TypeScript WatchedTaskList interface
type WatchedTaskList struct {
	Tasks   TaskList
	Release func()
}

// ValidateTask checks if a function is a valid task handler
// This mirrors the TypeScript isValidTask function
func ValidateTask(fn interface{}) bool {
	if handler, ok := fn.(TaskHandler); ok && handler != nil {
		return true
	}
	if task, ok := fn.(Task); ok && task != nil {
		return true
	}
	return false
}

// WorkerSharedOptions contains common options for workers and worker pools
// This mirrors the TypeScript WorkerSharedOptions interface
type WorkerSharedOptions struct {
	PollInterval *int           `json:"pollInterval,omitempty"` // How long to wait between polling (milliseconds)
	Logger       *logger.Logger `json:"logger,omitempty"`       // How should messages be logged
}

// SharedOptions contains common options for pools, workers, and utils
// This mirrors the TypeScript SharedOptions interface
type SharedOptions struct {
	Logger           *logger.Logger `json:"logger,omitempty"`           // How should messages be logged
	Schema           *string        `json:"schema,omitempty"`           // PostgreSQL schema to use
	ConnectionString *string        `json:"connectionString,omitempty"` // PostgreSQL connection string
	MaxPoolSize      *int           `json:"maxPoolSize,omitempty"`      // Maximum size of PostgreSQL pool
}

// WorkerInterface represents a job worker
// This mirrors the TypeScript Worker interface
type WorkerInterface interface {
	Nudge() bool
	GetWorkerID() string
	Release() error
	GetPromise() <-chan error
	GetActiveJob() *Job
}

// WorkerPoolInterface represents a worker pool
// This mirrors the TypeScript WorkerPool interface
type WorkerPoolInterface interface {
	Release() error
	GracefulShutdown(message string) error
	GetPromise() <-chan error
}

// RunnerInterface represents a task runner
// This mirrors the TypeScript Runner interface
type RunnerInterface interface {
	Stop() error
	AddJob(ctx context.Context, identifier string, payload interface{}, spec ...TaskSpec) error
	GetPromise() <-chan error
}

// WorkerUtilsInterface for working with Graphile Worker
// This mirrors the TypeScript WorkerUtils interface
type WorkerUtilsInterface interface {
	GetLogger() *logger.Logger
	WithPgClient(ctx context.Context, callback func(tx pgx.Tx) error) error
	AddJob(ctx context.Context, identifier string, payload interface{}, spec ...TaskSpec) error
	Release() error
	Migrate(ctx context.Context) error
	CompleteJobs(ctx context.Context, ids []string) ([]*Job, error)
	PermanentlyFailJobs(ctx context.Context, ids []string, reason *string) ([]*Job, error)
	RescheduleJobs(ctx context.Context, ids []string, options RescheduleJobsOptions) ([]*Job, error)
}

// RescheduleJobsOptions contains options for rescheduling jobs
// This mirrors options in TypeScript rescheduleJobs method
type RescheduleJobsOptions struct {
	RunAt       *string `json:"runAt,omitempty"`       // Schedule time
	Priority    *int    `json:"priority,omitempty"`    // Job priority
	Attempts    *int    `json:"attempts,omitempty"`    // Reset attempt count
	MaxAttempts *int    `json:"maxAttempts,omitempty"` // New maximum attempts
}
