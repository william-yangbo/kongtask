package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// RunTaskListOnceOptions represents options for running tasks once (compatibility)
type RunTaskListOnceOptions struct {
	Schema   string         // Database schema (default: "graphile_worker")
	Logger   *logger.Logger // Logger instance
	WorkerID string         // Worker ID (auto-generated if not provided)
}

// RunTaskListOnce runs all available jobs once and then exits (compatibility function)
// This is a compatibility wrapper around Worker.RunOnce()
func RunTaskListOnce(ctx context.Context, tasks map[string]TaskHandler, pool *pgxpool.Pool, options RunTaskListOnceOptions) error {
	// Validate required parameters
	if pool == nil {
		return fmt.Errorf("database pool cannot be nil")
	}
	if tasks == nil {
		return fmt.Errorf("tasks map cannot be nil")
	}

	// Set defaults
	if options.Schema == "" {
		options.Schema = "graphile_worker"
	}
	if options.Logger == nil {
		options.Logger = logger.DefaultLogger
	}
	if options.WorkerID == "" {
		options.WorkerID = fmt.Sprintf("worker-once-%d", time.Now().UnixNano())
	}

	// Create a worker
	worker := NewWorker(pool, options.Schema, WithLogger(options.Logger), WithWorkerID(options.WorkerID))

	// Register all tasks
	for taskName, handler := range tasks {
		worker.RegisterTask(taskName, handler)
	}

	// Run once
	return worker.RunOnce(ctx)
}
