package worker

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// RunnerOptions represents options for the runner (parity with graphile-worker RunnerOptions)
type RunnerOptions struct {
	// Database connection options (exactly one must be provided)
	ConnectionString string        `json:"connectionString,omitempty"` // Database connection string
	PgPool           *pgxpool.Pool `json:"-"`                          // Existing database pool

	// Task configuration (exactly one must be provided)
	TaskList      map[string]TaskHandler `json:"-"`                       // Map of task name to handler
	TaskDirectory string                 `json:"taskDirectory,omitempty"` // Directory containing task files

	// Optional configuration
	Schema   string         `json:"schema,omitempty"`   // Database schema (default: "graphile_worker")
	Logger   *logger.Logger `json:"-"`                  // Logger instance
	WorkerID string         `json:"workerId,omitempty"` // Custom worker ID
}

// RunOnce runs all available jobs once and then exits (parity with graphile-worker runOnce)
// This function validates options and runs jobs exactly once, matching graphile-worker behavior
func RunOnce(ctx context.Context, options RunnerOptions) error {
	// Validate task configuration - exactly one of taskList or taskDirectory must be provided
	if options.TaskList == nil && options.TaskDirectory == "" {
		return fmt.Errorf("you must specify either `options.taskList` or `options.taskDirectory`")
	}
	if options.TaskList != nil && options.TaskDirectory != "" {
		return fmt.Errorf("exactly one of either `taskDirectory` or `taskList` should be set")
	}

	// Validate database connection - exactly one connection method must be provided
	hasConnectionString := options.ConnectionString != ""
	hasPgPool := options.PgPool != nil
	hasDatabaseURL := os.Getenv("DATABASE_URL") != ""
	hasPgDatabase := os.Getenv("PGDATABASE") != ""

	if !hasConnectionString && !hasPgPool && !hasDatabaseURL && !hasPgDatabase {
		return fmt.Errorf("you must either specify `pgPool` or `connectionString`, or you must make the `DATABASE_URL` or `PG*` environmental variables available")
	}
	if hasConnectionString && hasPgPool {
		return fmt.Errorf("both `pgPool` and `connectionString` are set, at most one of these options should be provided")
	}

	// Get or create database pool
	var pool *pgxpool.Pool

	if options.PgPool != nil {
		pool = options.PgPool
	} else {
		var err error
		pool, err = createDatabasePool(ctx, options.ConnectionString)
		if err != nil {
			return fmt.Errorf("failed to create database pool: %w", err)
		}
		defer pool.Close()
	}

	// Set defaults
	schema := options.Schema
	if schema == "" {
		schema = "graphile_worker"
	}

	workerLogger := options.Logger
	if workerLogger == nil {
		workerLogger = logger.DefaultLogger
	}

	// For now, we only support TaskList (TaskDirectory support would require file system task loading)
	if options.TaskDirectory != "" {
		return fmt.Errorf("taskDirectory is not yet supported, please use TaskList")
	}

	// Run tasks once using RunTaskListOnce
	runOptions := RunTaskListOnceOptions{
		Schema:   schema,
		Logger:   workerLogger,
		WorkerID: options.WorkerID,
	}

	return RunTaskListOnce(ctx, options.TaskList, pool, runOptions)
}
