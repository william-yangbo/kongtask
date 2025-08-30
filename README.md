# KongTask

[![Go Report Card](https://goreportcard.com/badge/github.com/william-yangbo/kongtask)](https://goreportcard.com/report/github.com/william-yangbo/kongtask)
[![MIT license](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/william-yangbo/kongtask.svg)](https://pkg.go.dev/github.com/william-yangbo/kongtask)

A high-performance Go implementation of [graphile-worker](https://github.com/graphile/worker), providing background job processing for PostgreSQL databases. Run jobs (e.g. sending emails, processing images, generating reports) "in the background" so that your HTTP response/application code is not held up.

## Overview

KongTask is a job queue for PostgreSQL running on Go, designed as a high-performance alternative to the original Node.js graphile-worker. It maintains strict compatibility with graphile-worker's database schema while providing:

- **Native Go Performance**: ~700+ jobs/second processing speed
- **Low Latency**: ~50ms average job processing latency
- **PostgreSQL Native**: Uses PostgreSQL's LISTEN/NOTIFY for real-time job notifications
- **Schema Compatible**: Drop-in replacement for graphile-worker databases
- **Production Ready**: Comprehensive test suite with TestContainers

## Features

- ✅ **High Performance**: 700+ jobs/second with low latency processing
- ✅ **Database Migrations**: Identical schema setup as graphile-worker
- ✅ **PostgreSQL Integration**: Using pgx/v5 for high-performance database operations
- ✅ **TestContainers Support**: Comprehensive testing with real PostgreSQL containers
- ✅ **CLI Interface**: Command-line tool for migrations and job processing
- ✅ **Job Processing**: Task registration and execution system
- ✅ **Worker System**: Background job execution with configurable concurrency
- ✅ **Low Latency**: Uses `LISTEN`/`NOTIFY` for immediate job notifications
- ✅ **Parallel Processing**: Uses `SKIP LOCKED` for high-performance job fetching
- ✅ **Automatic Retries**: Exponential back-off retry mechanism
- ✅ **Job Queues**: Named queues for serialized job execution
- ✅ **Future Scheduling**: Schedule jobs to run at specific times

## Quickstart: CLI

### Add kongtask to your project:

```bash
go install github.com/william-yangbo/kongtask/cmd/kongtask@latest
```

### Run database migrations:

```bash
# Set your database connection
export DATABASE_URL="postgres://user:pass@localhost/mydb"

# Run migrations to set up the job queue schema
kongtask migrate
```

### Schedule a job:

Connect to your database and run the following SQL:

```sql
SELECT graphile_worker.add_job('send_email', json_build_object(
  'to', 'user@example.com',
  'subject', 'Welcome!',
  'body', 'Thanks for signing up'
));
```

### Create and run workers:

Create task handlers in your Go application:

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"

    "github.com/william-yangbo/kongtask/internal/worker"
    "github.com/jackc/pgx/v5/pgxpool"
)

type EmailPayload struct {
    To      string `json:"to"`
    Subject string `json:"subject"`
    Body    string `json:"body"`
}

func main() {
    // Connect to database
    pool, err := pgxpool.New(context.Background(), "postgres://user:pass@localhost/mydb")
    if err != nil {
        log.Fatal(err)
    }
    defer pool.Close()

    // Create worker pool
    workerPool, err := worker.NewPool(pool, &worker.Options{
        Concurrency: 5,
        PollInterval: 1000, // ms
    })
    if err != nil {
        log.Fatal(err)
    }

    // Register task handlers
    workerPool.RegisterTask("send_email", func(ctx context.Context, job *worker.Job) error {
        var payload EmailPayload
        if err := json.Unmarshal(job.Payload, &payload); err != nil {
            return err
        }

        fmt.Printf("Sending email to %s: %s\n", payload.To, payload.Subject)
        // Your email sending logic here
        return nil
    })

    // Start processing jobs
    if err := workerPool.Start(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

### Success!

You should see the worker output processing your email job. That's it!

## Quickstart: Library

Instead of using the CLI, you can embed kongtask directly in your Go application:

```go
package main

import (
    "context"
    "encoding/json"
    "log"

    "github.com/william-yangbo/kongtask/internal/worker"
    "github.com/william-yangbo/kongtask/internal/migrate"
    "github.com/jackc/pgx/v5/pgxpool"
)

func main() {
    ctx := context.Background()

    // Connect to database
    pool, err := pgxpool.New(ctx, "postgres://user:pass@localhost/mydb")
    if err != nil {
        log.Fatal(err)
    }
    defer pool.Close()

    // Run migrations
    if err := migrate.Migrate(ctx, pool); err != nil {
        log.Fatal(err)
    }

    // Create and configure worker pool
    workerPool, err := worker.NewPool(pool, &worker.Options{
        Concurrency:  5,
        PollInterval: 1000,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Register task handlers
    workerPool.RegisterTask("process_image", func(ctx context.Context, job *worker.Job) error {
        var payload map[string]interface{}
        if err := json.Unmarshal(job.Payload, &payload); err != nil {
            return err
        }

        log.Printf("Processing image: %v", payload)
        // Your image processing logic here
        return nil
    })

    // Add jobs programmatically
    err = workerPool.AddJob(ctx, "process_image", map[string]interface{}{
        "imageUrl": "https://example.com/image.jpg",
        "filters":  []string{"resize", "crop"},
    })
    if err != nil {
        log.Printf("Failed to add job: %v", err)
    }

    // Start worker pool
    if err := workerPool.Start(ctx); err != nil {
        log.Fatal(err)
    }

    // Graceful shutdown with workerPool.Shutdown(ctx)
}
```

## Requirements

PostgreSQL 12+ and Go 1.22+.

If your database doesn't already include the `pgcrypto` and `uuid-ossp` extensions, KongTask will automatically install them into the public schema for you.

## Installation

```bash
go install github.com/william-yangbo/kongtask/cmd/kongtask@latest
```

Or build from source:

```bash
git clone https://github.com/william-yangbo/kongtask.git
cd kongtask
go build -o bin/kongtask ./cmd/kongtask
```

## CLI Usage

KongTask manages its own database schema (`graphile_worker`). Just point kongtask at your database and it handles migrations automatically:

```bash
kongtask migrate -c "postgres://localhost/mydb"
```

The following CLI options are available:

```bash
Options:
  --help, -h              Show help
  --version, -v           Show version number
  --connection, -c        Database connection string, defaults to 'DATABASE_URL' envvar
  --schema                Database schema name (default: "graphile_worker")
  --silent                Suppress migration output
```

## Library Usage

KongTask can be used as a library inside your Go application. It exposes the `worker.NewPool(options)` function for creating worker pools.

### Worker Pool Options

- `Concurrency`: Number of jobs to run concurrently (default: 1)
- `PollInterval`: How long to wait between polling for jobs in milliseconds (default: 2000)
- `Logger`: Custom logger interface (optional)
- `Schema`: Database schema name (default: "graphile_worker")

### Example: Basic Worker Pool

```go
package main

import (
    "context"
    "log"

    "github.com/william-yangbo/kongtask/internal/worker"
    "github.com/jackc/pgx/v5/pgxpool"
)

func main() {
    pool, err := pgxpool.New(context.Background(),
        "postgres://postgres:postgres@localhost:5432/mydb")
    if err != nil {
        log.Fatal(err)
    }
    defer pool.Close()

    workerPool, err := worker.NewPool(pool, &worker.Options{
        Concurrency:  4,
        PollInterval: 1000,
    })
    if err != nil {
        log.Fatal(err)
    }

    workerPool.RegisterTask("my_task", func(ctx context.Context, job *worker.Job) error {
        log.Printf("Processing job %d with payload: %s", job.ID, string(job.Payload))
        return nil
    })

    // Start the worker pool
    if err := workerPool.Start(context.Background()); err != nil {
        log.Fatal(err)
    }

    // To stop: workerPool.Shutdown(context.Background())
}
```

## Creating Task Handlers

A task handler is a Go function that receives a job and processes it. It should return `nil` on success or an `error` on failure. Failed jobs are automatically retried with exponential backoff.

**IMPORTANT**: Your jobs should wait for all asynchronous work to be completed before returning, otherwise the job might be marked as successful prematurely.

**IMPORTANT**: Jobs are automatically retried on failure, so consider making them idempotent or splitting large jobs into smaller ones.

```go
// Basic task handler
func processOrder(ctx context.Context, job *worker.Job) error {
    var payload struct {
        OrderID int `json:"order_id"`
        UserID  int `json:"user_id"`
    }

    if err := json.Unmarshal(job.Payload, &payload); err != nil {
        return fmt.Errorf("invalid payload: %w", err)
    }

    // Process the order
    if err := processOrderLogic(payload.OrderID, payload.UserID); err != nil {
        return err // Job will be retried
    }

    return nil // Job completed successfully
}

// Register the handler
workerPool.RegisterTask("process_order", processOrder)
```

### Job Context and Helpers

Each task handler receives:

- `ctx context.Context` - for cancellation and timeouts
- `job *worker.Job` - containing:
  - `ID` - unique job identifier
  - `TaskIdentifier` - task name
  - `Payload` - JSON payload data
  - `QueueName` - queue this job belongs to
  - `CreatedAt` - when job was created
  - `Attempts` - number of attempts so far

### Adding Jobs with Options

You can schedule jobs with various options:

```go
// Basic job
err := workerPool.AddJob(ctx, "send_email", map[string]interface{}{
    "to": "user@example.com",
    "subject": "Welcome!",
})

// Job with options
err := workerPool.AddJobWithOptions(ctx, "send_email", payload, &worker.JobOptions{
    QueueName:   "email_queue",     // Run in specific queue
    RunAt:       time.Now().Add(time.Hour), // Schedule for later
    MaxAttempts: 5,                 // Custom retry limit
})
```

## Scheduling Jobs

You can schedule jobs directly in the database using SQL, or programmatically using the Go API.

### SQL Job Scheduling

The `graphile_worker.add_job` function accepts the following parameters:

- `identifier` - **required** task name to execute
- `payload` - JSON object with job data (default: empty object)
- `queue_name` - named queue for serialized execution (default: random)
- `run_at` - timestamp to run the job (default: now)
- `max_attempts` - retry limit (default: 25)

```sql
-- Basic job
SELECT graphile_worker.add_job('send_email', json_build_object(
    'to', 'user@example.com',
    'subject', 'Welcome!'
));

-- Job with all options
SELECT graphile_worker.add_job(
    'generate_report',                           -- task identifier
    json_build_object('user_id', 123),          -- payload
    'reports',                                   -- queue name
    NOW() + INTERVAL '1 hour',                  -- run at
    10                                          -- max attempts
);

-- Using named parameters
SELECT graphile_worker.add_job(
    'send_reminder',
    run_at := NOW() + INTERVAL '2 days'
);
```

### Database Triggers

Schedule jobs automatically from database triggers:

```sql
-- Trigger function for new user registrations
CREATE FUNCTION new_user_trigger() RETURNS trigger AS $$
BEGIN
    PERFORM graphile_worker.add_job('send_welcome_email', json_build_object(
        'user_id', NEW.id,
        'email', NEW.email
    ));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql VOLATILE;

-- Apply trigger to users table
CREATE TRIGGER user_registration_jobs
    AFTER INSERT ON users
    FOR EACH ROW
    EXECUTE FUNCTION new_user_trigger();
```

### Generic Trigger Function

A reusable trigger function for multiple tables:

```sql
CREATE FUNCTION trigger_job() RETURNS trigger AS $$
BEGIN
    PERFORM graphile_worker.add_job(TG_ARGV[0], json_build_object(
        'schema', TG_TABLE_SCHEMA,
        'table', TG_TABLE_NAME,
        'op', TG_OP,
        'id', (CASE WHEN TG_OP = 'DELETE' THEN OLD.id ELSE NEW.id END)
    ));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql VOLATILE;

-- Use with multiple tables
CREATE TRIGGER process_payment
    AFTER INSERT ON payments
    FOR EACH ROW
    EXECUTE FUNCTION trigger_job('process_payment');

CREATE TRIGGER user_updated
    AFTER UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION trigger_job('sync_user_data');
```

## Performance

KongTask is designed for high performance while maintaining simplicity. It achieves:

- **High Throughput**: 700+ jobs per second on modest hardware
- **Low Latency**: ~50ms average job processing latency
- **Efficient Polling**: Uses PostgreSQL's `LISTEN`/`NOTIFY` for real-time job notifications
- **Optimized Fetching**: Uses `SKIP LOCKED` for high-performance concurrent job fetching
- **Horizontal Scaling**: Multiple worker instances can run concurrently

### Performance Testing

Run the included performance test suite:

```bash
cd perftest
go test -v
```

This reveals performance metrics including:

- Job processing throughput (jobs/second)
- End-to-end latency measurements
- Memory usage patterns
- Startup/shutdown times

Sample results on modern hardware:

- **Bulk Processing**: 700+ jobs/second with 10 concurrent workers
- **Average Latency**: 50ms from job scheduling to execution
- **Startup Time**: <5ms for worker initialization

## Job Retry and Exponential Backoff

KongTask uses the same exponential backoff formula as graphile-worker: `exp(least(10, attempt))` seconds.

This provides reasonable delays for temporary failures:

| Attempt | Delay | Total Delay |
| ------- | ----- | ----------- |
| 1       | 2.7s  | 2.7s        |
| 2       | 7.4s  | 10.1s       |
| 3       | 20.1s | 30.2s       |
| 4       | 54.6s | 1.4m        |
| 5       | 2.5m  | 3.9m        |
| 10+     | 6.1h  | 9.7h+       |

After ~4 hours, attempts are made every ~6 hours until the maximum attempt count (default: 25) is reached.

## Database Schema

The migration creates the following schema, compatible with graphile-worker:

### Tables

- `migrations` - Track applied schema migrations
- `job_queues` - Queue configuration and statistics
- `jobs` - Individual job records with payload and metadata

### Functions

- `add_job(identifier, payload, queue_name, run_at, max_attempts)` - Schedule a new job
- `get_job(worker_id)` - Claim and retrieve a job for processing
- `complete_job(worker_id, job_id)` - Mark job as completed successfully
- `fail_job(worker_id, job_id, message)` - Mark job as failed with error message

## Uninstallation

To remove KongTask and all job data from your database:

```sql
DROP SCHEMA graphile_worker CASCADE;
```

**Warning**: This will delete all job data permanently.

## Development

### Requirements

- Go 1.22+
- PostgreSQL 12+
- Docker (for testing with TestContainers)

### Building from Source

```bash
git clone https://github.com/william-yangbo/kongtask.git
cd kongtask
go mod tidy
go build -o bin/kongtask ./cmd/kongtask
```

### Testing

Run the comprehensive test suite:

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run performance tests
cd perftest && go test -v

# Run specific test packages
go test ./internal/migrate/...
go test ./internal/worker/...
```

The test suite uses TestContainers to automatically start PostgreSQL instances for realistic testing.

### Project Structure

```
kongtask/
├── cmd/kongtask/              # CLI application
│   └── main.go
├── internal/
│   ├── migrate/               # Database migration system
│   │   ├── sql/              # SQL migration files (embedded)
│   │   └── migrate.go        # Migration logic
│   ├── worker/               # Job processing system
│   │   ├── pool.go           # Worker pool management
│   │   ├── worker.go         # Individual worker logic
│   │   └── job.go           # Job structures and processing
│   └── testutil/             # Test utilities and helpers
├── perftest/                 # Performance testing suite
│   ├── performance_test.go   # Comprehensive performance tests
│   ├── init.sql             # Test data initialization
│   └── README.md            # Performance testing documentation
├── docs/                     # Documentation
├── go.mod
├── go.sum
└── README.md
```

### Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Make your changes and add tests
4. Ensure tests pass: `go test ./...`
5. Commit your changes: `git commit -m 'Add amazing feature'`
6. Push to the branch: `git push origin feature/amazing-feature`
7. Open a Pull Request

### Code Style

- Follow standard Go formatting: `go fmt ./...`
- Run linters: `golangci-lint run`
- Add tests for new functionality
- Update documentation as needed

## Compatibility with graphile-worker

KongTask maintains strict compatibility with graphile-worker:

- **Database Schema**: 100% compatible with graphile-worker v0.1.0+
- **Job Processing**: Identical job lifecycle and state management
- **SQL Functions**: Matching function signatures and behavior
- **Migration System**: Compatible migration patterns
- **Performance**: Superior performance while maintaining compatibility

You can use KongTask as a drop-in replacement for graphile-worker in existing PostgreSQL databases.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- [graphile-worker](https://github.com/graphile/worker) - Original TypeScript implementation
- [pgx](https://github.com/jackc/pgx) - PostgreSQL driver for Go
- [TestContainers](https://www.testcontainers.org/) - Integration testing framework
