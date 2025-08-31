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
