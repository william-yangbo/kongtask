# KongTask

KongTask is a high-performance job queue implementation for Go, inspired by [graphile-worker](https://github.com/graphile/worker). It provides PostgreSQL-backed job processing with excellent performance and reliability.

## Features

- 🚀 **High Performance**: Process 1,700+ jobs per second
- 🔒 **Reliable**: PostgreSQL-backed job persistence and ACID guarantees
- 🎯 **Compatible**: API compatibility with graphile-worker v0.4.0+
- 🔧 **Flexible**: Support for job scheduling, retries, and custom task handlers
- 🛡️ **Secure**: Cryptographically secure worker ID generation (v0.5.0+ improvement)
- 📊 **Observable**: Comprehensive logging and metrics support

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
    // Connect to PostgreSQL
    pool, err := pgxpool.New(context.Background(), "postgres://user:password@localhost/dbname")
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
        Concurrency: 4,
        Schema:      "graphile_worker",
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

KongTask maintains API compatibility with graphile-worker:

- ✅ Job queue operations (add_job, get_job, complete_job, fail_job)
- ✅ Task scheduling and retry logic
- ✅ Database schema compatibility
- ✅ Worker pool management
- ✅ Performance characteristics

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests (`make test-all`)
5. Run linters (`make lint`)
6. Commit your changes (`git commit -m 'Add amazing feature'`)
7. Push to the branch (`git push origin feature/amazing-feature`)
8. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Inspired by [graphile-worker](https://github.com/graphile/worker)
- Built with [pgx](https://github.com/jackc/pgx) for PostgreSQL connectivity
- Thanks to the Go community for excellent tooling and libraries
🔒 **Security**: GitHub Advanced Security enabled with CodeQL + gosec scanning
