# KongTask

[![Go Report Card](https://goreportcard.com/badge/github.com/william-yangbo/kongtask)](https://goreportcard.com/report/github.com/william-yangbo/kongtask)
[![MIT license](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/william-yangbo/kongtask.svg)](https://pkg.go.dev/github.com/william-yangbo/kongtask)

**High-performance PostgreSQL job queue for Go** - 700+ jobs/second with 50ms latency.

KongTask is a Go port of [graphile-worker](https://github.com/graphile/worker) with 100% schema compatibility and superior performance.

## Features

- 🚀 **High Performance**: 700+ jobs/sec, 50ms latency
- 🔄 **Drop-in Compatible**: Works with existing graphile-worker databases
- ⚡ **Real-time Processing**: PostgreSQL LISTEN/NOTIFY
- 🔁 **Auto Retries**: Exponential backoff (25 attempts)
- 📅 **Job Scheduling**: Run now or schedule for later
- 🎯 **Named Queues**: Serialize related jobs
- 🛠️ **Production Ready**: Battle-tested with comprehensive test suite

## Quick Start

### 1. Install

```bash
go install github.com/william-yangbo/kongtask/cmd/kongtask@latest
```

### 2. Setup Database

```bash
export DATABASE_URL="postgres://user:pass@localhost/mydb"
kongtask migrate
```

### 3. Add a Job

```sql
SELECT graphile_worker.add_job('send_email', '{"to": "user@example.com"}');
```

### 4. Process Jobs

```go
package main

import (
    "context"
    "log"
    "github.com/william-yangbo/kongtask/internal/worker"
    "github.com/jackc/pgx/v5/pgxpool"
)

func main() {
    pool, _ := pgxpool.New(context.Background(), "postgres://user:pass@localhost/mydb")
    defer pool.Close()

    w, _ := worker.NewPool(pool, &worker.Options{Concurrency: 5})

    w.RegisterTask("send_email", func(ctx context.Context, job *worker.Job) error {
        log.Printf("Sending email: %s", job.Payload)
        // Your email logic here
        return nil
    })

    w.Start(context.Background()) // Blocks and processes jobs
}
```

**That's it!** Your jobs will be processed as they're added.

## Performance

| Metric         | KongTask      | graphile-worker  |
| -------------- | ------------- | ---------------- |
| **Throughput** | 700+ jobs/sec | 500-600 jobs/sec |
| **Latency**    | 50ms avg      | 80-100ms avg     |
| **Memory**     | 15MB baseline | 25MB baseline    |
| **Startup**    | <5ms          | ~50ms            |

Run benchmarks: `cd perftest && go test -v`

## Documentation

- 📖 [API Reference](docs/api.md) - Complete API documentation
- 🚀 [Advanced Usage](docs/advanced.md) - Error handling, queues, scheduling
- 🗃️ [Database Schema](docs/schema.md) - Tables, functions, compatibility
- ⚡ [Performance Guide](docs/performance.md) - Benchmarks and optimization

## Installation

### Go Install

```bash
go install github.com/william-yangbo/kongtask/cmd/kongtask@latest
```

### Build from Source

```bash
git clone https://github.com/william-yangbo/kongtask.git
cd kongtask
go build -o bin/kongtask ./cmd/kongtask
```

### Requirements

- PostgreSQL 12+
- Go 1.22+

## Basic Examples

### CLI Usage

```bash
# Initialize schema
kongtask migrate -c "postgres://localhost/mydb"
```

### Library Usage

```go
// Create worker pool
w, _ := worker.NewPool(pool, &worker.Options{Concurrency: 10})

// Register task handler
w.RegisterTask("send_email", func(ctx context.Context, job *worker.Job) error {
    // Process the job
    return sendEmail(job.Payload)
})

// Add job
w.AddJob(ctx, "send_email", map[string]string{"to": "user@example.com"})
```

### SQL Job Creation

```sql
SELECT graphile_worker.add_job('send_email', '{"to": "user@example.com"}');
```

## Contributing

1. Fork the repository
2. Create feature branch: `git checkout -b feature/amazing-feature`
3. Add tests: `go test ./...`
4. Commit changes: `git commit -m 'Add amazing feature'`
5. Push to branch: `git push origin feature/amazing-feature`
6. Open Pull Request

## Compatibility

KongTask maintains 100% compatibility with graphile-worker:

- ✅ Same database schema
- ✅ Identical job lifecycle
- ✅ Matching SQL functions
- ✅ Drop-in replacement

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Acknowledgments

- [graphile-worker](https://github.com/graphile/worker) - Original TypeScript implementation
- [pgx](https://github.com/jackc/pgx) - PostgreSQL driver for Go
