# KongTask

High-performance task queue implementation for Go, compatible with [graphile-worker](https://github.com/graphile/worker) core API. Built on PostgreSQL for reliable task processing.

## Features

- 🚀 **High Performance**: 1700+ tasks/second with 4 workers
- 🔒 **Reliable**: PostgreSQL ACID guarantees with automatic retries
- 🗓️ **Cron Scheduling**: Supports cron-format scheduled tasks
- ⚡ **Easy to Use**: Simple API design for quick onboarding

## Quick Start

### Installation

```bash
go get github.com/william-yangbo/kongtask
```

### Basic Example

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
    pool, err := pgxpool.New(context.Background(),
        "postgres://user:password@localhost/dbname")
    if err != nil {
        log.Fatal(err)
    }
    defer pool.Close()

    // Define task handlers
    tasks := map[string]worker.TaskHandler{
        "send_email": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
            log.Printf("Sending email: %s", payload)
            return nil
        },
    }

    // Create worker pool
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

    workerPool.Wait()
}
```

## Configuration

Multiple database connection methods supported:

```bash
# Method 1: Environment variable
export DATABASE_URL="postgres://user:password@localhost/dbname"

# Method 2: PostgreSQL standard environment variables
export PGHOST=localhost PGPORT=5432 PGDATABASE=mydb PGUSER=user PGPASSWORD=pass

# Method 3: Command line
./kongtask worker --database-url "postgres://user:password@localhost/dbname"
```

## Documentation

For detailed information, see:

- [API Reference](docs/API_REFERENCE.md) - Complete API documentation
- [Cron Scheduling](docs/CRONTAB.md) - Cron scheduling guide
- [Environment Variables](docs/ENVIRONMENT.md) - Environment configuration guide
- [Deployment](docs/DEPLOYMENT.md) - Production deployment guide

## Development

```bash
# Build
make build

# Test
make test

# Start test database
make db-setup
```

## License

MIT License - see [LICENSE](LICENSE) file for details
