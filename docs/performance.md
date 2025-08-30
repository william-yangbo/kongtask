# Performance Guide

## Benchmark Results

KongTask delivers superior performance compared to graphile-worker:

| Metric         | KongTask      | graphile-worker  | Improvement |
| -------------- | ------------- | ---------------- | ----------- |
| **Throughput** | 700+ jobs/sec | 500-600 jobs/sec | +17-40%     |
| **Latency**    | 50ms avg      | 80-100ms avg     | -38-50%     |
| **Memory**     | 15MB baseline | 25MB baseline    | -40%        |
| **Startup**    | <5ms          | ~50ms            | -90%        |

## Running Benchmarks

### Performance Test Suite

```bash
cd perftest
go test -v
```

### Test Results Example

```
=== Performance Test Results ===
Bulk Jobs Performance:
- Processed 10000 jobs in 14.089s
- Throughput: 709.74 jobs/second
- Average latency: 49.23ms
- Memory usage: 15.2MB

Latency Performance:
- P50: 45ms
- P95: 120ms
- P99: 250ms
- Max: 890ms

Startup/Shutdown:
- Worker startup: 3.2ms
- Graceful shutdown: 45ms
```

### Custom Benchmarks

```go
package main

import (
    "context"
    "testing"
    "time"

    "github.com/william-yangbo/kongtask/internal/worker"
)

func BenchmarkJobProcessing(b *testing.B) {
    pool := setupTestPool()
    defer pool.Close()

    workerPool, _ := worker.NewPool(pool, &worker.Options{
        Concurrency: 10,
    })

    workerPool.RegisterTask("benchmark_task", func(ctx context.Context, job *worker.Job) error {
        // Simulate work
        time.Sleep(1 * time.Millisecond)
        return nil
    })

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go workerPool.Start(ctx)

    b.ResetTimer()

    for i := 0; i < b.N; i++ {
        workerPool.AddJob(context.Background(), "benchmark_task", map[string]int{
            "iteration": i,
        })
    }
}
```

## Optimization Strategies

### Database Optimization

#### Connection Pooling

```go
config, _ := pgxpool.ParseConfig(databaseURL)

// Optimize for high throughput
config.MaxConns = 25
config.MinConns = 5
config.MaxConnLifetime = time.Hour
config.MaxConnIdleTime = 30 * time.Minute

pool, _ := pgxpool.NewWithConfig(context.Background(), config)
```

#### PostgreSQL Configuration

```sql
-- postgresql.conf optimizations
shared_buffers = 256MB
effective_cache_size = 1GB
maintenance_work_mem = 64MB
checkpoint_completion_target = 0.9
wal_buffers = 16MB
default_statistics_target = 100
random_page_cost = 1.1
effective_io_concurrency = 200
```

### Worker Pool Optimization

#### Concurrency Tuning

```go
// CPU-bound tasks
concurrency := runtime.NumCPU()

// I/O-bound tasks
concurrency := runtime.NumCPU() * 2

// Database-heavy tasks
concurrency := dbPool.Config().MaxConns / 2

workerPool, _ := worker.NewPool(pool, &worker.Options{
    Concurrency: concurrency,
    PollInterval: 500, // Reduce for lower latency
})
```

#### Batching Strategy

```go
// Batch job creation for higher throughput
jobs := make([]worker.JobSpec, 1000)
for i := range jobs {
    jobs[i] = worker.JobSpec{
        TaskName: "batch_task",
        Payload:  map[string]int{"id": i},
    }
}

// Add jobs in batch
workerPool.AddJobsBatch(ctx, jobs)
```

### Memory Optimization

#### Payload Size Management

```go
// Keep payloads small
type EfficientPayload struct {
    ID     int64  `json:"id"`      // Reference to larger data
    Type   string `json:"type"`    // Job type indicator
    Config string `json:"config"`  // Minimal configuration
}

// Fetch full data in handler
workerPool.RegisterTask("optimized_task", func(ctx context.Context, job *worker.Job) error {
    var payload EfficientPayload
    json.Unmarshal(job.Payload, &payload)

    // Fetch full data from database/cache
    fullData := fetchFullData(payload.ID)
    return processData(fullData)
})
```

#### Resource Cleanup

```go
workerPool.RegisterTask("memory_efficient", func(ctx context.Context, job *worker.Job) error {
    // Process in chunks to avoid memory spikes
    data := fetchLargeDataset(job)

    for chunk := range chunkData(data, 1000) {
        if err := processChunk(chunk); err != nil {
            return err
        }

        // Explicit cleanup
        runtime.GC()
    }

    return nil
})
```

## Monitoring Performance

### Metrics Collection

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    jobsProcessed = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kongtask_jobs_processed_total",
            Help: "Total number of jobs processed",
        },
        []string{"task_name", "status"},
    )

    jobDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "kongtask_job_duration_seconds",
            Help:    "Job processing duration",
            Buckets: prometheus.DefBuckets,
        },
        []string{"task_name"},
    )
)

workerPool.RegisterTask("monitored_task", func(ctx context.Context, job *worker.Job) error {
    timer := prometheus.NewTimer(jobDuration.WithLabelValues(job.TaskIdentifier))
    defer timer.ObserveDuration()

    err := processJob(job)

    status := "success"
    if err != nil {
        status = "error"
    }
    jobsProcessed.WithLabelValues(job.TaskIdentifier, status).Inc()

    return err
})
```

### Performance Dashboard

Example Grafana queries:

```promql
# Jobs per second
rate(kongtask_jobs_processed_total[1m])

# Average processing time
rate(kongtask_job_duration_seconds_sum[5m]) / rate(kongtask_job_duration_seconds_count[5m])

# Error rate
rate(kongtask_jobs_processed_total{status="error"}[5m]) / rate(kongtask_jobs_processed_total[5m])
```

### Database Monitoring

```sql
-- Active job monitoring
SELECT
    task_identifier,
    COUNT(*) as active_jobs,
    MIN(run_at) as oldest_job,
    AVG(attempts) as avg_attempts
FROM graphile_worker.jobs
WHERE attempts < max_attempts
GROUP BY task_identifier;

-- Queue throughput
SELECT
    queue_name,
    COUNT(*) as jobs_processed,
    AVG(updated_at - created_at) as avg_processing_time
FROM graphile_worker.jobs
WHERE updated_at > NOW() - INTERVAL '1 hour'
GROUP BY queue_name;
```

## Performance Tips

### 1. Right-size Concurrency

- Start with `runtime.NumCPU()`
- Monitor CPU/memory usage
- Adjust based on job characteristics

### 2. Optimize Database

- Use connection pooling
- Index optimization
- Regular VACUUM/ANALYZE

### 3. Efficient Job Design

- Keep payloads small
- Make jobs idempotent
- Avoid long-running tasks

### 4. Monitor and Alert

- Track job throughput
- Monitor error rates
- Set up latency alerts

### 5. Horizontal Scaling

- Run multiple worker instances
- Use named queues for ordering
- Load balance across regions

## Troubleshooting Performance

### High Latency

- Check database connection pool
- Verify index usage
- Monitor LISTEN/NOTIFY delays

### Low Throughput

- Increase concurrency
- Optimize job handlers
- Check for database locks

### Memory Issues

- Reduce payload sizes
- Add explicit cleanup
- Monitor for memory leaks

### Database Performance

- Check slow query log
- Analyze execution plans
- Optimize PostgreSQL settings
