# KongTask

KongTask is a high-performance job queue implementation for Go, providing core API compatibility with [graphile-worker](https://github.com/graphile/worker) v0.8.1. It delivers PostgreSQL-backed job processing with excellent performance and reliability.

> **Compatibility Status**: Core features aligned with graphile-worker v0.8.1. Some TypeScript-specific features are not supported (see limitations below).

## Features

- 🚀 **High Performance**: Process 1,700+ jobs per second
- 🔒 **Reliable**: PostgreSQL-backed job persistence and ACID guarantees
- 🎯 **Compatible**: Core API compatibility with graphile-worker v0.8.1 (see limitations below)
- 🔧 **Flexible**: Support for job scheduling, retries, and custom task handlers
- 🛡️ **Secure**: Cryptographically secure worker ID generation
- 📊 **Observable**: Comprehensive logging and metrics support
- 🏷️ **Forbidden Flags**: Runtime job filtering for complex rate limiting and selective processing
- 🔀 **Task De-duplication**: Via unique `job_key` for preventing duplicate work
- ⚡ **Optimized Indexing**: Enhanced database performance with partial indexes (v0.8.1 features)

## Recent Updates (v0.8.1 Alignment)

KongTask has been updated to align with graphile-worker v0.8.1, bringing significant performance improvements:

- 🚀 **Enhanced Database Performance**: New optimized index `jobs_priority_run_at_id_locked_at_without_failures_idx` with partial indexing
- 📊 **Reduced Index Size**: Partial indexes exclude failed jobs (`attempts < max_attempts`) for better performance
- 🔧 **Migration 000006**: Automatically applied database schema updates for performance optimization
- 🔗 **Full Compatibility**: Maintains backward compatibility while adding latest performance enhancements

The performance improvements are particularly beneficial for high-throughput scenarios and large job queues.

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

## Database Configuration

KongTask supports multiple ways to configure your PostgreSQL database connection (in order of priority):

1. **Direct connection string** (in your Go code)
2. **`DATABASE_URL` environment variable**
3. **PostgreSQL standard environment variables** (`PG*` envvars)

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

### Command Line Options

```bash
# Using connection flag
./kongtask worker --connection "postgres://user:password@localhost/dbname"

# Using database-url flag
./kongtask worker --database-url "postgres://user:password@localhost/dbname"
```

> **Note**: When using PostgreSQL environment variables, at least `PGDATABASE` must be set. Other variables will use PostgreSQL defaults if not specified.

## Development

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

KongTask is built with several key components:

- **Worker**: Individual job processor with secure ID generation
- **WorkerPool**: Manages multiple workers with graceful shutdown
- **Job Queue**: PostgreSQL-backed job persistence and scheduling
- **Task Handlers**: User-defined functions for job processing
- **Helpers**: Utility functions for job management

## Compatibility

KongTask provides core API compatibility with graphile-worker v0.8.1:

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

**Development Status**: Core job queue functionality fully compatible with graphile-worker v0.8.1. Language-specific features (like dynamic task loading) are implemented using Go patterns rather than direct TypeScript equivalents.

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

## Changelog

### v0.8.1 Alignment (September 2025)

- 🚀 **Performance**: Synchronized with graphile-worker v0.8.1 performance optimizations
- 📊 **Database**: Added migration 000006 with optimized partial indexing
- ⚡ **Index**: Enhanced `jobs_priority_run_at_id_locked_at_without_failures_idx` for better query performance
- 🔧 **Schema**: Updated database schema exports to reflect latest optimizations
- ✅ **Tests**: Updated integration tests to support new migration count

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
