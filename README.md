# KongTask

KongTask## Recent Updates (v0.10.0)

KongTask v0.10.0 brings comprehensive error handling improvements and infrastructure enhancements, fully aligning with graphile-worker's latest stability features:

- 🛡️ **Enhanced Error Handling**: Complete synchronization with graphile-worker commit e714bd0 for production-ready stability
- 🔧 **PostgreSQL Pool Monitoring**: Background health monitoring equivalent to node-postgres pool error handling
- 🚫 **Signal Processing**: Removed SIGPIPE from signal handling for Node.js behavior alignment
- 📡 **Database Notifications**: Enhanced notification listener with improved error recovery and reconnection logic
- 🧹 **Resource Management**: Comprehensive cleanup functions and connection lifecycle improvements
- 📊 **Error Recovery**: Graceful error recovery with automatic reconnection capabilities and enhanced logging

The enhanced error handling provides enterprise-grade stability and monitoring for production deployments.

## Previous Updates (v0.9.0)

KongTask v0.9.0 brought major cron scheduling capabilities and logging improvements, aligning with the latest graphile-worker features:

- 🗓️ **Complete Cron Scheduling**: File-based cron scheduling with intelligent loading and status reporting
- 🔇 **Reduced Logging Noise**: Debug-level clock skew logging and enhanced task handler best practices
- 🎯 **CLI Consistency**: Unified `--crontab` parameter interface matching graphile-worker conventions
- 📚 **Enhanced Documentation**: Comprehensive guides for cron scheduling and logging best practices
- 🧠 **Intelligent Loading**: Smart crontab file detection with detailed status feedback

The cron scheduling system provides production-ready task automation with full compatibility to graphile-worker patterns.performance job queue implementation for Go, providing core API compatibility with [graphile-worker](https://github.com/graphile/worker) v0.10.0. It delivers PostgreSQL-backed job processing with excellent performance and reliability.

> **Compatibility Status**: Core features aligned with graphile-worker v0.9.0, including complete cron scheduling system and enhanced error handling from v0.10.0 (commit e714bd0). Some TypeScript-specific features are not supported (see limitations below).

## Features

- 🚀 **High Performance**: Process 1,700+ jobs per second
- 🔒 **Reliable**: PostgreSQL-backed job persistence and ACID guarantees
- 🎯 **Compatible**: Core API compatibility with graphile-worker v0.10.0 (see limitations below)
- 🔧 **Flexible**: Support for job scheduling, retries, and custom task handlers
- 🛡️ **Secure**: Cryptographically secure worker ID generation
- 📊 **Observable**: Comprehensive logging and metrics support
- 📡 **Events System**: Complete worker and job lifecycle monitoring with EventBus
- 🗓️ **Cron Scheduling**: File-based recurring task scheduling with intelligent loading (v0.9.0)
- 🏷️ **Forbidden Flags**: Runtime job filtering for complex rate limiting and selective processing
- 🔀 **Task De-duplication**: Via unique `job_key` for preventing duplicate work
- ⚡ **Optimized Indexing**: Enhanced database performance with partial indexes (v0.8.1 features)

## Recent Updates (v0.9.0)

KongTask v0.9.0 brings major cron scheduling capabilities and logging improvements, aligning with the latest graphile-worker features:

- �️ **Complete Cron Scheduling**: File-based cron scheduling with intelligent loading and status reporting
- � **Reduced Logging Noise**: Debug-level clock skew logging and enhanced task handler best practices
- 🎯 **CLI Consistency**: Unified `--crontab` parameter interface matching graphile-worker conventions
- � **Enhanced Documentation**: Comprehensive guides for cron scheduling and logging best practices
- � **Intelligent Loading**: Smart crontab file detection with detailed status feedback

The cron scheduling system provides production-ready task automation with full compatibility to graphile-worker patterns.

## Documentation

For detailed information, see our comprehensive documentation:

- 📋 **[API Reference](docs/API_REFERENCE.md)** - Complete API documentation with types and examples
- ⏰ **[Crontab Guide](docs/CRONTAB.md)** - Comprehensive cron scheduling documentation
- 🛠️ **[Task Handlers](docs/TASK_HANDLERS.md)** - Go best practices for creating robust task handlers
- 📝 **[Custom Logging](docs/LOGGING.md)** - Integration guide for popular Go logging libraries
- 🌍 **[Environment Variables](docs/ENVIRONMENT.md)** - Comprehensive environment variable configuration guide
- 🚀 **[Deployment](docs/DEPLOYMENT.md)** - Production deployment and operations guide

## Quick Start

### Installation

```bash
go get github.com/william-yangbo/kongtask
```

### Basic Usage

```go
package main

import (
    "context"
    "encoding/json"
    "log"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/william-yangbo/kongtask/pkg/worker"
)

func main() {
    // Connect to PostgreSQL (local)
    pool, err := pgxpool.New(context.Background(), "postgres://user:password@localhost/dbname")
    // For a remote database (TLS), prefer `?ssl=true` in the connection string
    // e.g. "postgres://user:pass@host:port/dbname?ssl=true"
    if err != nil {
        log.Fatal(err)
    }
    defer pool.Close()

    // Define task handler
    tasks := map[string]worker.TaskHandler{
        "send_email": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
            // Process the email sending task
            log.Printf("Sending email: %s", payload)
            return nil
        },
    }

    // Create and run worker pool
    workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
        Concurrency:     4,
        Schema:          "graphile_worker",
        // Install signal handlers for graceful shutdown on SIGINT, SIGTERM, etc (v0.5.0+ feature)
        NoHandleSignals: false,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer workerPool.Release()

    // Add a job
    w := worker.NewWorker(pool, "graphile_worker")
    err = w.AddJob(context.Background(), "send_email", map[string]string{
        "to":      "user@example.com",
        "subject": "Welcome!",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Wait for completion
    workerPool.Wait()
}
```

## API Reference

> **📖 For complete API documentation, see [API Reference](docs/API_REFERENCE.md)**

### TaskSpec

The `TaskSpec` struct provides comprehensive job scheduling options:

```go
type TaskSpec struct {
    // The queue to run this task under (only specify if you want jobs
    // in this queue to run serially). (Default: nil)
    QueueName *string `json:"queueName,omitempty"`

    // A time to schedule this task to run in the future. (Default: now)
    RunAt *time.Time `json:"runAt,omitempty"`

    // Jobs are executed in numerically ascending order of priority
    // (jobs with a numerically smaller priority are run first). (Default: 0)
    Priority *int `json:"priority,omitempty"`

    // How many retries should this task get? (Default: 25)
    MaxAttempts *int `json:"maxAttempts,omitempty"`

    // Unique identifier for the job, used to replace, update or remove
    // it later if needed. (Default: nil)
    JobKey *string `json:"jobKey,omitempty"`

    // Controls the behavior of `jobKey` when a matching job is found:
    // - "replace" (default): all attributes will be updated
    // - "preserve_run_at": all attributes except 'run_at' will be updated
    // - "unsafe_dedupe": only adds job if no existing job with matching key exists
    JobKeyMode *string `json:"jobKeyMode,omitempty"`

    // Flags for the job, can be used to dynamically filter which jobs
    // can and cannot run at runtime. (Default: nil)
    Flags []string `json:"flags,omitempty"`
}
```

### Adding Jobs

```go
// Simple job
err := workerUtils.AddJob(ctx, "task_name", payload)

// Job with scheduling options
err := workerUtils.QuickAddJob(ctx, "task_name", payload, worker.TaskSpec{
    QueueName:   stringPtr("high_priority"),
    RunAt:       timePtr(time.Now().Add(1 * time.Hour)),
    MaxAttempts: intPtr(10),
    JobKey:      stringPtr("unique-job-id"),
    JobKeyMode:  stringPtr(worker.JobKeyModeReplace),
    Priority:    intPtr(1),
})
```

### Helper Functions

```go
// Utility functions for pointer creation
func stringPtr(s string) *string { return &s }
func intPtr(i int) *int { return &i }
func timePtr(t time.Time) *time.Time { return &t }
```

## Database Configuration

KongTask supports multiple ways to configure your PostgreSQL database connection (in order of priority):

1. **Direct connection string** (in your Go code)
2. **`DATABASE_URL` environment variable**
3. **PostgreSQL standard environment variables** (`PG*` envvars)

> **📝 For comprehensive environment variable configuration, see [Environment Variables Guide](docs/ENVIRONMENT.md)**

### Using Environment Variables

#### DATABASE_URL

```bash
export DATABASE_URL="postgres://user:password@localhost/dbname"
./kongtask worker
```

#### PostgreSQL Standard Environment Variables

```bash
export PGHOST=localhost
export PGPORT=5432
export PGDATABASE=kongtask_dev
export PGUSER=myuser
export PGPASSWORD=mypassword
./kongtask worker
```

#### KongTask-Specific Environment Variables

```bash
# New KONGTASK_ prefix (recommended)
export KONGTASK_DEBUG=true
export KONGTASK_CONCURRENCY=4
export KONGTASK_MAX_POOL_SIZE=20
export KONGTASK_LOG_LEVEL=debug

# Legacy GRAPHILE_WORKER_ prefix (still supported)
export GRAPHILE_WORKER_SCHEMA=custom_schema
export GRAPHILE_WORKER_DEBUG=1
./kongtask worker
```

### Command Line Options

```bash
# Using connection flag
./kongtask worker --connection "postgres://user:password@localhost/dbname"

# Using database-url flag
./kongtask worker --database-url "postgres://user:password@localhost/dbname"

# Using custom crontab file (default: "./crontab")
./kongtask worker --crontab /path/to/custom.crontab

# Complete example with multiple options
./kongtask worker \
  --database-url "postgres://user:password@localhost/dbname" \
  --crontab ./my-schedules.txt \
  --jobs 4 \
  --schema graphile_worker
```

> **Note**: When using PostgreSQL environment variables, at least `PGDATABASE` must be set. Other variables will use PostgreSQL defaults if not specified.

## Development

> **🛠️ For detailed task handler patterns and best practices, see [Task Handlers Guide](docs/TASK_HANDLERS.md)**

### Prerequisites

- Go 1.22 or later
- PostgreSQL 14+
- Docker (for tests)

### Building

```bash
# Build the project
make build

# Build command-line tools
make build-cmd
```

### Testing

```bash
# Run unit tests
make test

# Run integration tests
make test-integration

# Run performance tests
make test-perf

# Run all tests
make test-all
```

### Code Quality

```bash
# Run linters
make lint

# Format code
make fmt

# Run security checks
make security

# Run all CI checks locally
make ci-test
```

### Database Setup

```bash
# Start test database
make db-setup

# Stop test database
make db-teardown

# Export database schema for verification
make db-dump

# Export custom schema
make db-dump-custom SCHEMA=my_schema OUTPUT=internal/migrate/schema/my_schema.sql
```

### Database Migrations

New database migrations must be accompanied by an updated schema dump for verification and documentation. This ensures migration files correctly produce the expected database structure.

Schema files are stored in `internal/migrate/schema/` alongside migration files for better organization and version control.

To create a new migration:

1. Add your migration file (e.g., `internal/migrate/sql/000006.sql`)
2. Run the migration on a test database
3. Export the updated schema: `make db-dump`
4. Review the generated `internal/migrate/schema/graphile_worker.sql`
5. Verify the schema matches your expectations
6. Commit both the migration file and updated schema dump

Using Docker for schema export:

```bash
# Start a clean PostgreSQL instance
docker run -e POSTGRES_HOST_AUTH_METHOD=trust -d -p 5432:5432 postgres:16

# Run migrations and export schema
PGUSER=postgres PGHOST=localhost make db-dump
```

## Performance

KongTask delivers excellent performance:

- **Throughput**: 1,700+ jobs/second with 4 workers
- **Latency**: ~50ms average job processing latency
- **Memory**: Efficient memory usage with minimal allocations
- **Concurrency**: Excellent scaling with multiple workers

## Architecture

> **📝 For custom logging integration examples, see [Logging Guide](docs/LOGGING.md)**  
> **🚀 For production deployment guidance, see [Deployment Guide](docs/DEPLOYMENT.md)**

KongTask is built with several key components:

- **Worker**: Individual job processor with secure ID generation
- **WorkerPool**: Manages multiple workers with graceful shutdown
- **Job Queue**: PostgreSQL-backed job persistence and scheduling
- **Task Handlers**: User-defined functions for job processing
- **Helpers**: Utility functions for job management

## Job Key and JobKeyMode

KongTask supports advanced job management through job keys and JobKeyMode, enabling sophisticated patterns like debouncing, throttling, and deduplication.

### Job Key Basics

Jobs can be assigned a unique `JobKey` to enable replacing, updating, or removing them later:

```go
// Add a job with a key
err := workerUtils.QuickAddJob(ctx, "send_email", payload, worker.TaskSpec{
    JobKey: stringPtr("user-123-welcome-email"),
})
```

### JobKeyMode Options

When adding a job with an existing `JobKey`, the `JobKeyMode` controls the behavior:

#### 1. `replace` (Default) - Debouncing Behavior

Overwrites the unlocked job with new values, including `run_at`. Perfect for debouncing - delaying execution until there have been no events for a certain period.

```go
err := workerUtils.QuickAddJob(ctx, "send_email", payload, worker.TaskSpec{
    JobKey:     stringPtr("user-123-notification"),
    JobKeyMode: stringPtr(worker.JobKeyModeReplace),
    RunAt:      timePtr(time.Now().Add(5 * time.Minute)), // Delay 5 minutes
})
```

#### 2. `preserve_run_at` - Throttling Behavior

Updates all job attributes except `run_at`, preserving the original execution time. Ideal for throttling - executing at most once over a given period.

```go
err := workerUtils.QuickAddJob(ctx, "send_digest", payload, worker.TaskSpec{
    JobKey:     stringPtr("user-123-daily-digest"),
    JobKeyMode: stringPtr(worker.JobKeyModePreserveRunAt),
})
```

#### 3. `unsafe_dedupe` - Dangerous Deduplication

Only inserts if no existing job with the same key exists (including locked or failed jobs). **Use with extreme caution** as events may not result in execution.

```go
err := workerUtils.QuickAddJob(ctx, "one_time_setup", payload, worker.TaskSpec{
    JobKey:     stringPtr("user-123-setup"),
    JobKeyMode: stringPtr(worker.JobKeyModeUnsafeDedupe),
})
```

### Convenience Methods

KongTask provides convenience methods for common JobKeyMode patterns:

```go
// Debouncing (replace mode)
err := workerUtils.AddJobWithReplace(ctx, "send_email", payload, "user-123-notification")

// Throttling (preserve_run_at mode)
err := workerUtils.AddJobWithPreserveRunAt(ctx, "rate_limited_api", payload, "api-call-batch-1")

// Deduplication (unsafe_dedupe mode)
err := workerUtils.AddJobWithUnsafeDedupe(ctx, "initialization", payload, "app-init-task")
```

### JobKeyMode Algorithm

The complete behavior when a job with an existing `JobKey` is found:

1. **No existing job found**: Creates a new job with the specified attributes
2. **`unsafe_dedupe` mode**: Returns the existing job without changes
3. **Existing job is locked**: Clears the existing job's key, sets it to max attempts, and creates a new job
4. **Existing job has failed**: Resets attempts to 0, clears errors, and updates all attributes (including `run_at`)
5. **`preserve_run_at` mode**: Updates all attributes except `run_at`
6. **Default behavior**: Updates all attributes including `run_at`

### Best Practices

- **Use descriptive job keys**: Include task type and relevant IDs (`"email-user-123-welcome"`)
- **Choose the right mode**:
  - `replace` for debouncing user actions
  - `preserve_run_at` for rate limiting and throttling
  - `unsafe_dedupe` only when you're certain about the consequences
- **Handle locked jobs**: Remember that locked jobs will result in new job creation (except with `unsafe_dedupe`)

## Compatibility

KongTask provides core API compatibility with graphile-worker v0.10.0:

### ✅ **Supported Features**

- ✅ **Database Schema**: Complete SQL migration alignment (000001-000006)
- ✅ **Performance Optimizations**: Enhanced database indexes with partial indexing (v0.8.1 features)
- ✅ **Job Queue Operations**: add_job, get_job, complete_job, fail_job functions
- ✅ **Admin Functions**: completeJobs, permanentlyFailJobs, rescheduleJobs (v0.4.0+ features)
- ✅ **Signal Handling Control**: NoHandleSignals option for custom signal management (v0.5.0+ features)
- ✅ **Task Scheduling**: Job scheduling, retry logic, and priority support
- ✅ **Forbidden Flags**: Runtime job filtering for complex rate limiting and selective processing
- ✅ **Job Flags**: Dynamic job filtering capabilities with static and function-based configuration
- ✅ **Worker Management**: Worker pool management with graceful shutdown
- ✅ **Performance**: Comparable throughput and latency characteristics
- ✅ **API Surface**: WorkerUtils, TaskSpec, and helper function compatibility

### ❌ **Known Limitations**

- ❌ **Dynamic Task Loading**: TypeScript's dynamic task loading from directories/modules is not supported
- ❌ **Runtime Task Registration**: Go's static typing requires tasks to be registered at compile time
- ❌ **Module System Integration**: No equivalent to TypeScript's import/require for task discovery

### 🔧 **Go-Specific Approach**

Instead of dynamic loading, KongTask uses static task registration:

```go
// Define tasks at compile time
tasks := map[string]worker.TaskHandler{
    "send_email": emailHandler,
    "process_payment": paymentHandler,
}
```

**Development Status**: Core job queue functionality fully compatible with graphile-worker v0.10.0. Language-specific features (like dynamic task loading) are implemented using Go patterns rather than direct TypeScript equivalents.

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests (`make test-all`)
5. Run linters (`make lint`)
6. Commit your changes (`git commit -m 'Add amazing feature'`)
7. Push to the branch (`git push origin feature/amazing-feature`)
8. Open a Pull Request

## Forbidden flags

When a job is created (or updated via `job_key`), you may set its `flags` to a
list of strings. When the worker is run, you may configure `forbiddenFlags` to
indicate that jobs with any of the given flags should not be executed.

```go
// Static forbidden flags
worker := worker.NewWorker(pool, schema,
    worker.WithForbiddenFlags([]string{"maintenance", "rate-limited"}),
)

// Dynamic forbidden flags function
forbiddenFlagsFn := func() ([]string, error) {
    // Check external systems, cache, database, etc.
    if isMaintenanceMode() {
        return []string{"maintenance"}, nil
    }
    return nil, nil
}

worker := worker.NewWorker(pool, schema,
    worker.WithForbiddenFlagsFn(forbiddenFlagsFn),
)
```

The `forbiddenFlags` option can be:

- `nil` (no filtering)
- an array of strings (`[]string{"flag1", "flag2"}`)
- a function returning `([]string, error)` that will be called each time a
  worker looks for a job to run

If `forbiddenFlags` is a function, KongTask will invoke it each time a
worker looks for a job to run, and will skip over any job that has any flag
returned by your function. You should ensure that your function resolves
quickly; it's advised that you maintain a cache you update periodically (e.g.
once a second) rather than always calculating on the fly, or use pub/sub or a
similar technique to maintain the forbidden flags list.

This feature enables complex rate limiting, maintenance mode controls, feature
flags, and other advanced job filtering scenarios at runtime.

## Events System

KongTask provides a comprehensive events system for monitoring worker and job lifecycle, fully compatible with graphile-worker's event interface (v0.4.0+ alignment).

### Event Types

The event system supports the following events:

- **`worker:getJob:start`** - When a worker is about to ask the database for a job to execute
- **`job:complete`** - When a job has finished executing and the result (success or failure) has been written back to the database
- **`job:start`** - When a job is retrieved and starts executing
- **`job:success`** - When a job completes successfully
- **`job:error`** - When a job throws an error
- **`job:failed`** - When a job fails permanently (emitted after job:error when appropriate)
- **`worker:create`** - When a worker is created
- **`worker:release`** - When a worker release is requested
- **`worker:stop`** - When a worker stops (normally after a release)

### Usage Example

```go
package main

import (
    "context"
    "log"

    "github.com/william-yangbo/kongtask/pkg/events"
    "github.com/william-yangbo/kongtask/pkg/worker"
)

func main() {
    // Create EventBus for monitoring
    eventBus := events.NewEventBus()

    // Listen to worker events
    eventBus.On(events.WorkerGetJobStart, func(data interface{}) {
        if eventData, ok := data.(events.WorkerGetJobStartData); ok {
            log.Printf("Worker %s is looking for a job", eventData.WorkerID)
        }
    })

    // Listen to job completion events
    eventBus.On(events.JobComplete, func(data interface{}) {
        if eventData, ok := data.(events.JobCompleteData); ok {
            log.Printf("Job %s completed by worker %s",
                eventData.Job.ID, eventData.WorkerID)
        }
    })

    // Use EventBus in worker configuration
    workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
        Concurrency: 4,
        Events:      eventBus, // Pass EventBus for event monitoring
    })
    if err != nil {
        log.Fatal(err)
    }
    defer workerPool.Release()
}
```

### Event Data Structures

Each event provides relevant context data:

- **WorkerGetJobStart**: `{ WorkerID: string }`
- **JobComplete**: `{ WorkerID: string, Job: *Job, Error: error }` (includes error field for both success and failure)
- **JobStart**: `{ WorkerID: string, Job: *Job }`
- **JobSuccess**: `{ WorkerID: string, Job: *Job }`
- **JobError**: `{ WorkerID: string, Job: *Job, Error: error }`
- **JobFailed**: `{ WorkerID: string, Job: *Job, Error: error }`

This event system enables comprehensive monitoring, metrics collection, logging, and integration with external monitoring systems.

## Error Codes

KongTask uses the same error codes as graphile-worker for consistency:

- `GWBID` - Task identifier is too long (max length: 128).
- `GWBQN` - Job queue name is too long (max length: 128).
- `GWBJK` - Job key is too long (max length: 512).
- `GWBMA` - Job maximum attempts must be at least 1.
- `GWBKM` - Invalid job_key_mode value, expected 'replace', 'preserve_run_at' or 'unsafe_dedupe'.

## Changelog

### v0.10.1 (September 2025)

- 🌍 **Environment Variable Improvements**: Enhanced environment variable handling with centralized management
  - **New KONGTASK\_\* prefix**: Introduced `KONGTASK_DEBUG`, `KONGTASK_CONCURRENCY`, `KONGTASK_MAX_POOL_SIZE`, `KONGTASK_LOG_LEVEL` environment variables
  - **Priority System**: Clear priority order - CLI flags > KONGTASK*\* > GRAPHILE_WORKER*\* > defaults
  - **Backward Compatibility**: Full support for existing `GRAPHILE_WORKER_*` environment variables
  - **Boolean Handling**: Flexible boolean parsing (true/1/yes/on and false/0/no/off, case-insensitive)
  - **Validation & Fallbacks**: Intelligent validation with graceful fallback to defaults for invalid values
  - **Centralized Management**: New `pkg/env` package for consistent environment variable handling across the application
- 📚 **Documentation**: New comprehensive [Environment Variables Guide](docs/ENVIRONMENT.md)
  - Complete reference for all supported environment variables
  - Migration guide from graphile-worker environment variables
  - Development vs production configuration examples
  - Environment variable priority and validation documentation

### v0.10.0 (September 2025)

- 🛡️ **Enhanced Error Handling**: Complete synchronization with graphile-worker commit e714bd0
  - **Signal Handling**: Removed SIGPIPE from signal processing (aligned with Node.js behavior)
  - **Database Notifications**: Enhanced notification listener with improved error recovery and reconnection logic
  - **PostgreSQL Pool Monitoring**: Implemented health monitoring equivalent to node-postgres pool error handling
  - **Resource Cleanup**: Added comprehensive cleanup functions and error state management
  - **Connection Lifecycle**: Improved connection management with proper error handling and recovery patterns
- 🔧 **Infrastructure**: Full error handling alignment provides production-ready stability
  - Background health monitoring for database pool connections
  - Graceful error recovery with automatic reconnection capabilities
  - Enhanced logging for PostgreSQL connection issues and recovery attempts
  - Comprehensive error event emission for monitoring and debugging
- 🎯 **API Compatibility**: Core feature alignment updated to graphile-worker v0.10.0
  - Complete error handling parity with upstream graphile-worker
  - Enhanced stability for enterprise production deployments
  - Comprehensive error detection and recovery mechanisms

### v0.9.0 (September 2025)

- 🗓️ **Cron Scheduling**: Complete cron scheduling system implementation
  - Synchronize with graphile-worker commit ace64ac cron improvements
  - CLI parameter alignment: `--crontab-file` → `--crontab` for consistency
  - Intelligent crontab file loading with status-aware logging
  - Enhanced documentation with custom crontab path examples
- 🔇 **Logging Improvements**: Reduced noise and enhanced debugging (commit 71d22e9 alignment)
  - Debug-level logging for cron clock skew events (reduced production noise)
  - Enhanced task handler logging best practices documentation
  - Recommended use of `helpers.Logger` over direct logging calls
  - Comprehensive logging level control and debug message handling
- 📚 **Documentation**: Major improvements across multiple areas
  - Updated CLI documentation with new `--crontab` parameter examples
  - Complete cron scheduling guide in `docs/CRONTAB.md`
  - Enhanced `docs/LOGGING.md` with task handler best practices
  - README CLI examples with multiple parameter combinations
- 🎯 **Interface Consistency**: Full alignment with graphile-worker patterns
  - Unified CLI interface design matching upstream conventions
  - Intelligent error handling with detailed diagnostic messages
  - Consistent parameter naming and behavior across components

### v0.8.1 Alignment (September 2025)

- 🚀 **Performance**: Synchronized with graphile-worker v0.8.1 performance optimizations
- 📊 **Database**: Added migration 000006 with optimized partial indexing
- ⚡ **Index**: Enhanced `jobs_priority_run_at_id_locked_at_without_failures_idx` for better query performance
- 🔧 **Schema**: Updated database schema exports to reflect latest optimizations
- ✅ **Tests**: Updated integration tests to support new migration count
- 📡 **Events**: Complete events system implementation (commit 92f4b3d alignment)
  - Added `worker:getJob:start` and `job:complete` events
  - Full EventBus implementation with typed event data
  - Compatible with graphile-worker v0.4.0+ event interface
  - Comprehensive event monitoring for worker and job lifecycle
- 🔑 **JobKeyMode**: Added complete JobKeyMode support (commit e7ab91e alignment)
  - Added migration 000007 with 9-parameter `add_job` function
  - Implemented `replace`, `preserve_run_at`, and `unsafe_dedupe` modes
  - Added convenience methods: `AddJobWithReplace`, `AddJobWithPreserveRunAt`, `AddJobWithUnsafeDedupe`
  - Complete interface alignment with TypeScript implementation
  - Comprehensive test coverage for all JobKeyMode behaviors

### Previous Versions

- **v0.5.0 Compatibility**: Initial feature parity with graphile-worker core functionality
- **Core Features**: Job queue operations, worker management, forbidden flags, and signal handling

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Inspired by [graphile-worker](https://github.com/graphile/worker)
- Built with [pgx](https://github.com/jackc/pgx) for PostgreSQL connectivity
- Thanks to the Go community for excellent tooling and libraries
  🔒 **Security**: GitHub Advanced Security enabled with CodeQL + gosec scanning
