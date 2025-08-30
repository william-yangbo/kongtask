# Advanced Usage

## Error Handling and Retries

KongTask automatically retries failed jobs using exponential backoff:

| Attempt | Delay | Total Delay |
| ------- | ----- | ----------- |
| 1       | 2.7s  | 2.7s        |
| 2       | 7.4s  | 10.1s       |
| 3       | 20.1s | 30.2s       |
| 4       | 54.6s | 1.4m        |
| 5       | 2.5m  | 3.9m        |
| 10+     | 6.1h  | 9.7h+       |

### Custom Error Handling

```go
workerPool.RegisterTask("risky_task", func(ctx context.Context, job *worker.Job) error {
    if err := doSomethingRisky(); err != nil {
        // This will trigger a retry
        return fmt.Errorf("task failed: %w", err)
    }
    return nil // Success - job completed
})
```

### Permanent Failures

Some errors should not be retried:

```go
import "github.com/william-yangbo/kongtask/internal/worker"

workerPool.RegisterTask("validate_data", func(ctx context.Context, job *worker.Job) error {
    if err := validateInput(job.Payload); err != nil {
        // Don't retry validation errors
        return worker.NewPermanentError(err)
    }
    return processData(job.Payload)
})
```

## Named Queues

Use named queues to serialize related jobs:

```go
// All jobs in "user_123" queue run sequentially
workerPool.AddJobWithOptions(ctx, "update_profile", payload, &worker.JobOptions{
    QueueName: "user_123",
})

workerPool.AddJobWithOptions(ctx, "send_notification", payload, &worker.JobOptions{
    QueueName: "user_123", // Will wait for update_profile to complete
})
```

## Job Scheduling

### Delayed Jobs

```go
// Schedule job for 1 hour from now
workerPool.AddJobWithOptions(ctx, "send_reminder", payload, &worker.JobOptions{
    RunAt: time.Now().Add(time.Hour),
})

// Schedule daily reports
workerPool.AddJobWithOptions(ctx, "daily_report", payload, &worker.JobOptions{
    RunAt: time.Now().Add(24 * time.Hour),
})
```

### Cron-like Scheduling

For recurring jobs, schedule the next occurrence from within the job:

```go
workerPool.RegisterTask("daily_backup", func(ctx context.Context, job *worker.Job) error {
    // Perform backup
    if err := performBackup(); err != nil {
        return err
    }

    // Schedule next backup
    return workerPool.AddJobWithOptions(ctx, "daily_backup", nil, &worker.JobOptions{
        RunAt: time.Now().Add(24 * time.Hour),
    })
})
```

## Monitoring and Observability

### Custom Logger

```go
import "log/slog"

logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

workerPool, err := worker.NewPool(pool, &worker.Options{
    Concurrency: 10,
    Logger:      logger,
})
```

### Metrics Collection

```go
workerPool.RegisterTask("tracked_task", func(ctx context.Context, job *worker.Job) error {
    start := time.Now()
    defer func() {
        duration := time.Since(start)
        metrics.RecordJobDuration("tracked_task", duration)
    }()

    return processJob(job)
})
```

### Health Checks

```go
func healthCheck(pool *worker.Pool) error {
    // Check if worker pool is healthy
    stats := pool.GetStats()
    if stats.ActiveWorkers == 0 {
        return errors.New("no active workers")
    }
    return nil
}
```

## Production Deployment

### Environment Variables

```bash
# Database connection
export DATABASE_URL="postgres://user:pass@host:5432/dbname?sslmode=require"

# Worker configuration
export WORKER_CONCURRENCY=20
export WORKER_POLL_INTERVAL=1000
export WORKER_SCHEMA="graphile_worker"
```

### Docker Deployment

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o kongtask ./cmd/kongtask

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/kongtask .
CMD ["./kongtask"]
```

### Kubernetes Example

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kongtask-worker
spec:
  replicas: 3
  selector:
    matchLabels:
      app: kongtask-worker
  template:
    metadata:
      labels:
        app: kongtask-worker
    spec:
      containers:
        - name: kongtask
          image: kongtask:latest
          env:
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: db-secret
                  key: url
            - name: WORKER_CONCURRENCY
              value: '10'
```

### Database Connection Pooling

```go
// Configure connection pool for production
config, err := pgxpool.ParseConfig(databaseURL)
if err != nil {
    log.Fatal(err)
}

// Production pool settings
config.MaxConns = 20
config.MinConns = 5
config.MaxConnLifetime = time.Hour
config.MaxConnIdleTime = time.Minute * 30

pool, err := pgxpool.NewWithConfig(context.Background(), config)
```

### Graceful Shutdown

```go
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle shutdown signals
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)

    go func() {
        <-c
        log.Println("Shutting down gracefully...")
        cancel()
    }()

    // Start worker
    if err := workerPool.Start(ctx); err != nil {
        log.Printf("Worker pool error: %v", err)
    }

    // Graceful shutdown with timeout
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()

    if err := workerPool.Shutdown(shutdownCtx); err != nil {
        log.Printf("Shutdown error: %v", err)
    }
}
```
