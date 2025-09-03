# Deployment and Operations Guide

This guide covers deployment, monitoring, and operational best practices for KongTask in production environments.

## Production Deployment

### 1. Database Setup

#### PostgreSQL Configuration

Minimum PostgreSQL version: **12+**

```sql
-- Create dedicated database
CREATE DATABASE kongtask_prod;

-- Create user with appropriate permissions
CREATE USER kongtask_worker WITH PASSWORD 'secure_password';
GRANT CONNECT ON DATABASE kongtask_prod TO kongtask_worker;

-- Grant schema permissions
\c kongtask_prod
GRANT USAGE, CREATE ON SCHEMA public TO kongtask_worker;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO kongtask_worker;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO kongtask_worker;

-- For custom schema
CREATE SCHEMA IF NOT EXISTS graphile_worker;
GRANT USAGE, CREATE ON SCHEMA graphile_worker TO kongtask_worker;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA graphile_worker TO kongtask_worker;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA graphile_worker TO kongtask_worker;
```

#### Connection Pool Configuration

```go
package main

import (
    "context"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

func NewProductionPool(databaseURL string) (*pgxpool.Pool, error) {
    config, err := pgxpool.ParseConfig(databaseURL)
    if err != nil {
        return nil, err
    }

    // Production connection pool settings
    config.MaxConns = 25                    // Maximum connections
    config.MinConns = 5                     // Minimum connections
    config.MaxConnLifetime = time.Hour      // Connection lifetime
    config.MaxConnIdleTime = time.Minute * 30 // Idle timeout
    config.HealthCheckPeriod = time.Minute   // Health check interval

    // Connection timeout settings
    config.ConnConfig.ConnectTimeout = time.Second * 10
    config.ConnConfig.RuntimeParams["statement_timeout"] = "30s"
    config.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "5min"

    return pgxpool.NewWithConfig(context.Background(), config)
}
```

### 2. Application Configuration

#### Environment Variables

```bash
# Database
DATABASE_URL="postgres://kongtask_worker:secure_password@localhost:5432/kongtask_prod?sslmode=require"

# Worker Configuration
WORKER_CONCURRENCY=10
WORKER_SCHEMA="graphile_worker"
WORKER_POLL_INTERVAL="2s"
WORKER_MAX_POOL_SIZE=20

# Logging
LOG_LEVEL="info"
LOG_FORMAT="json"

# Monitoring
METRICS_PORT=9090
HEALTH_CHECK_PORT=8080

# Security
TLS_CERT_FILE="/etc/ssl/certs/app.crt"
TLS_KEY_FILE="/etc/ssl/private/app.key"
```

#### Production Worker Configuration

```go
package main

import (
    "context"
    "encoding/json"
    "log"
    "os"
    "strconv"
    "time"

    "github.com/william-yangbo/kongtask/pkg/worker"
    "github.com/jackc/pgx/v5/pgxpool"
)

func main() {
    // Parse environment configuration
    config := ProductionConfig{
        DatabaseURL:  getEnv("DATABASE_URL", ""),
        Concurrency:  getEnvInt("WORKER_CONCURRENCY", 4),
        Schema:       getEnv("WORKER_SCHEMA", "graphile_worker"),
        PollInterval: getEnvDuration("WORKER_POLL_INTERVAL", 2*time.Second),
        MaxPoolSize:  getEnvInt("WORKER_MAX_POOL_SIZE", 10),
    }

    // Database connection
    pool, err := NewProductionPool(config.DatabaseURL)
    if err != nil {
        log.Fatal("Failed to connect to database:", err)
    }
    defer pool.Close()

    // Run migrations
    if err := worker.RunMigrations(context.Background(), pool, config.Schema); err != nil {
        log.Fatal("Failed to run migrations:", err)
    }

    // Setup monitoring
    logger := NewProductionLogger()
    events := NewProductionEventBus()

    // Configure worker pool
    workerPool, err := worker.RunTaskList(
        context.Background(),
        getTaskHandlers(),
        pool,
        worker.WorkerPoolOptions{
            Concurrency:  config.Concurrency,
            Schema:       config.Schema,
            PollInterval: config.PollInterval,
            MaxPoolSize:  config.MaxPoolSize,
            Logger:       logger,
            Events:       events,
            CronItems:    getCronItems(),
        },
    )
    if err != nil {
        log.Fatal("Failed to start worker pool:", err)
    }
    defer workerPool.Release()

    // Start health check server
    go startHealthCheckServer(":8080", pool)

    // Start metrics server
    go startMetricsServer(":9090")

    // Wait for completion
    if err := workerPool.Wait(); err != nil {
        log.Printf("Worker pool stopped with error: %v", err)
    }
}
```

### 3. Docker Deployment

#### Dockerfile

```dockerfile
# Multi-stage build
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o kongtask ./cmd/kongtask

# Production image
FROM alpine:latest

# Install ca-certificates for SSL connections
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy binary
COPY --from=builder /app/kongtask .

# Create non-root user
RUN addgroup -g 1001 -S kongtask && \
    adduser -u 1001 -S kongtask -G kongtask

USER kongtask

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

EXPOSE 8080 9090

CMD ["./kongtask"]
```

#### Docker Compose

```yaml
version: '3.8'

services:
  kongtask:
    build: .
    environment:
      - DATABASE_URL=postgres://kongtask_worker:password@postgres:5432/kongtask_prod
      - WORKER_CONCURRENCY=8
      - LOG_LEVEL=info
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - '8080:8080' # Health check
      - '9090:9090' # Metrics
    deploy:
      replicas: 2
      resources:
        limits:
          memory: 512M
          cpus: '0.5'
        reservations:
          memory: 256M
          cpus: '0.25'
    restart: unless-stopped
    healthcheck:
      test: ['CMD', 'wget', '--spider', '-q', 'http://localhost:8080/health']
      interval: 30s
      timeout: 10s
      retries: 3

  postgres:
    image: postgres:15-alpine
    environment:
      - POSTGRES_DB=kongtask_prod
      - POSTGRES_USER=kongtask_worker
      - POSTGRES_PASSWORD=password
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./init.sql:/docker-entrypoint-initdb.d/init.sql
    ports:
      - '5432:5432'
    healthcheck:
      test: ['CMD-SHELL', 'pg_isready -U kongtask_worker -d kongtask_prod']
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  postgres_data:
```

### 4. Kubernetes Deployment

#### Deployment Manifest

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kongtask-worker
  labels:
    app: kongtask-worker
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
          ports:
            - containerPort: 8080
              name: health
            - containerPort: 9090
              name: metrics
          env:
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: kongtask-secrets
                  key: database-url
            - name: WORKER_CONCURRENCY
              value: '8'
            - name: LOG_LEVEL
              value: 'info'
          resources:
            requests:
              memory: '256Mi'
              cpu: '250m'
            limits:
              memory: '512Mi'
              cpu: '500m'
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 30
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /ready
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          lifecycle:
            preStop:
              exec:
                command: ['/bin/sh', '-c', 'sleep 15']
---
apiVersion: v1
kind: Service
metadata:
  name: kongtask-service
spec:
  selector:
    app: kongtask-worker
  ports:
    - name: health
      port: 8080
      targetPort: 8080
    - name: metrics
      port: 9090
      targetPort: 9090
---
apiVersion: v1
kind: Secret
metadata:
  name: kongtask-secrets
type: Opaque
data:
  database-url: cG9zdGdyZXM6Ly9rb25ndGFza193b3JrZXI6cGFzc3dvcmRAcG9zdGdyZXM6NTQzMi9rb25ndGFza19wcm9k
```

#### ConfigMap for Cron Jobs

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kongtask-cron-config
data:
  cron.json: |
    [
      {
        "task": "cleanup_old_jobs",
        "identifier": "cleanup",
        "pattern": "0 2 * * *",
        "options": {
          "backfill_period": "1h",
          "max_attempts": 3
        },
        "payload": {
          "days_to_keep": 30
        }
      },
      {
        "task": "generate_reports",
        "identifier": "daily_reports",
        "pattern": "0 8 * * 1-5",
        "options": {
          "queue_name": "reports",
          "priority": 10
        },
        "payload": {
          "report_type": "daily"
        }
      }
    ]
```

## Monitoring and Observability

### 1. Health Checks

```go
package main

import (
    "context"
    "encoding/json"
    "net/http"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

type HealthStatus struct {
    Status    string            `json:"status"`
    Timestamp string            `json:"timestamp"`
    Checks    map[string]Check  `json:"checks"`
}

type Check struct {
    Status  string `json:"status"`
    Message string `json:"message,omitempty"`
}

func startHealthCheckServer(addr string, pool *pgxpool.Pool) {
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        status := checkHealth(pool)

        w.Header().Set("Content-Type", "application/json")
        if status.Status != "healthy" {
            w.WriteHeader(http.StatusServiceUnavailable)
        }

        json.NewEncoder(w).Encode(status)
    })

    http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
        // Quick readiness check
        if err := pool.Ping(context.Background()); err != nil {
            w.WriteHeader(http.StatusServiceUnavailable)
            return
        }
        w.WriteHeader(http.StatusOK)
    })

    http.ListenAndServe(addr, nil)
}

func checkHealth(pool *pgxpool.Pool) HealthStatus {
    status := HealthStatus{
        Timestamp: time.Now().UTC().Format(time.RFC3339),
        Checks:    make(map[string]Check),
    }

    // Database connectivity check
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := pool.Ping(ctx); err != nil {
        status.Checks["database"] = Check{
            Status:  "unhealthy",
            Message: err.Error(),
        }
        status.Status = "unhealthy"
    } else {
        status.Checks["database"] = Check{Status: "healthy"}
    }

    // Additional checks
    status.Checks["memory"] = checkMemoryUsage()
    status.Checks["disk"] = checkDiskUsage()

    // Determine overall status
    if status.Status != "unhealthy" {
        status.Status = "healthy"
        for _, check := range status.Checks {
            if check.Status != "healthy" {
                status.Status = "degraded"
                break
            }
        }
    }

    return status
}
```

### 2. Metrics Collection

```go
package main

import (
    "net/http"
    "time"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    jobsProcessed = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kongtask_jobs_processed_total",
            Help: "Total number of jobs processed",
        },
        []string{"task", "status"},
    )

    jobDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "kongtask_job_duration_seconds",
            Help: "Job execution duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"task"},
    )

    activeWorkers = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "kongtask_active_workers",
            Help: "Number of active workers",
        },
        []string{"pool"},
    )

    queueSize = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "kongtask_queue_size",
            Help: "Number of jobs in queue",
        },
        []string{"queue"},
    )
)

func init() {
    prometheus.MustRegister(jobsProcessed)
    prometheus.MustRegister(jobDuration)
    prometheus.MustRegister(activeWorkers)
    prometheus.MustRegister(queueSize)
}

func startMetricsServer(addr string) {
    http.Handle("/metrics", promhttp.Handler())
    http.ListenAndServe(addr, nil)
}

// Event bus integration for metrics
type MetricsEventBus struct {
    // Implementation
}

func (m *MetricsEventBus) On(event string, handler func(data interface{})) {
    switch event {
    case "job:complete":
        // Wrap handler to collect metrics
        wrappedHandler := func(data interface{}) {
            if jobData, ok := data.(JobCompleteData); ok {
                status := "success"
                if jobData.Error != nil {
                    status = "error"
                }

                jobsProcessed.WithLabelValues(
                    jobData.Job.TaskIdentifier,
                    status,
                ).Inc()
            }
            handler(data)
        }
        // Register wrapped handler
    }
}
```

### 3. Logging Configuration

```go
package main

import (
    "os"

    "github.com/sirupsen/logrus"
)

func NewProductionLogger() *logrus.Logger {
    logger := logrus.New()

    // Set log level from environment
    level, err := logrus.ParseLevel(os.Getenv("LOG_LEVEL"))
    if err != nil {
        level = logrus.InfoLevel
    }
    logger.SetLevel(level)

    // Use JSON formatter for production
    if os.Getenv("LOG_FORMAT") == "json" {
        logger.SetFormatter(&logrus.JSONFormatter{
            TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
            FieldMap: logrus.FieldMap{
                logrus.FieldKeyTime:  "timestamp",
                logrus.FieldKeyLevel: "level",
                logrus.FieldKeyMsg:   "message",
            },
        })
    }

    // Add common fields
    logger = logger.WithFields(logrus.Fields{
        "service": "kongtask",
        "version": "1.0.0",
    }).Logger

    return logger
}
```

## Performance Optimization

### 1. Database Tuning

```sql
-- PostgreSQL configuration for high-throughput workloads

-- Memory settings
shared_buffers = '256MB'                    -- 25% of RAM
effective_cache_size = '1GB'                -- 75% of RAM
work_mem = '64MB'                           -- Per operation
maintenance_work_mem = '256MB'              -- Maintenance operations

-- Connection settings
max_connections = 100                       -- Adjust based on load
shared_preload_libraries = 'pg_stat_statements'

-- WAL settings
wal_buffers = '16MB'
checkpoint_completion_target = 0.9
wal_writer_delay = '200ms'

-- Query tuning
random_page_cost = 1.1                      -- For SSD storage
effective_io_concurrency = 200              -- For SSD storage

-- Create indexes for performance
CREATE INDEX CONCURRENTLY idx_jobs_run_at_task ON graphile_worker.jobs(run_at, task_identifier)
WHERE locked_at IS NULL;

CREATE INDEX CONCURRENTLY idx_jobs_queue_priority ON graphile_worker.jobs(queue_name, priority)
WHERE locked_at IS NULL;

-- Analyze tables regularly
ANALYZE graphile_worker.jobs;
```

### 2. Worker Pool Sizing

```go
package main

import (
    "runtime"
    "time"
)

func calculateOptimalConcurrency() int {
    // Start with CPU count
    cpuCount := runtime.NumCPU()

    // For I/O bound tasks, can be higher than CPU count
    // For CPU bound tasks, should be close to CPU count
    return cpuCount * 2
}

func calculatePollInterval(jobVolume string) time.Duration {
    switch jobVolume {
    case "high":
        return 500 * time.Millisecond   // High throughput
    case "medium":
        return 2 * time.Second          // Balanced
    case "low":
        return 10 * time.Second         // Low resource usage
    default:
        return 2 * time.Second
    }
}
```

### 3. Connection Pool Optimization

```go
func optimizeConnectionPool(expectedConcurrency int) *pgxpool.Config {
    config, _ := pgxpool.ParseConfig(databaseURL)

    // Connection pool sizing
    // Rule: MaxConns >= Concurrency + Admin connections + Buffer
    config.MaxConns = int32(expectedConcurrency + 5)
    config.MinConns = int32(expectedConcurrency / 4)

    // Connection lifetime management
    config.MaxConnLifetime = 1 * time.Hour
    config.MaxConnIdleTime = 15 * time.Minute

    // Health checking
    config.HealthCheckPeriod = 1 * time.Minute

    return config
}
```

## Security Best Practices

### 1. Database Security

```sql
-- Principle of least privilege
REVOKE ALL ON DATABASE kongtask_prod FROM PUBLIC;
GRANT CONNECT ON DATABASE kongtask_prod TO kongtask_worker;

-- Schema-level permissions
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO kongtask_worker;

-- Table-level permissions (after migrations)
GRANT SELECT, INSERT, UPDATE, DELETE ON graphile_worker.jobs TO kongtask_worker;
GRANT SELECT, INSERT, UPDATE, DELETE ON graphile_worker.job_queues TO kongtask_worker;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA graphile_worker TO kongtask_worker;

-- Row-level security (if needed)
ALTER TABLE graphile_worker.jobs ENABLE ROW LEVEL SECURITY;
```

### 2. Application Security

```go
package main

import (
    "context"
    "crypto/tls"
    "time"
)

func secureTaskHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    // Input validation
    if len(payload) > 1024*1024 { // 1MB limit
        return errors.New("payload too large")
    }

    // Timeout protection
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    // Sanitize logging
    helpers.Logger.Info("Processing task", map[string]interface{}{
        "task_id": helpers.Job.ID,
        // Don't log sensitive payload data
    })

    return nil
}

func setupTLSConfig() *tls.Config {
    return &tls.Config{
        MinVersion:               tls.VersionTLS12,
        CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
        PreferServerCipherSuites: true,
        CipherSuites: []uint16{
            tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
            tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
            tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
        },
    }
}
```

## Troubleshooting

### 1. Common Issues

#### Database Connection Issues

```bash
# Check connection
psql "postgres://kongtask_worker:password@localhost:5432/kongtask_prod" -c "SELECT 1;"

# Check active connections
SELECT count(*) as active_connections FROM pg_stat_activity WHERE datname = 'kongtask_prod';

# Check for long-running queries
SELECT query, query_start, state FROM pg_stat_activity
WHERE datname = 'kongtask_prod' AND query_start < NOW() - INTERVAL '5 minutes';
```

#### Job Processing Issues

```sql
-- Check stuck jobs
SELECT id, task_identifier, locked_at, locked_by, attempts, last_error
FROM graphile_worker.jobs
WHERE locked_at IS NOT NULL AND locked_at < NOW() - INTERVAL '1 hour';

-- Check failed jobs
SELECT task_identifier, count(*) as failed_count
FROM graphile_worker.jobs
WHERE attempts >= max_attempts
GROUP BY task_identifier
ORDER BY failed_count DESC;

-- Reset stuck jobs
UPDATE graphile_worker.jobs
SET locked_at = NULL, locked_by = NULL
WHERE locked_at < NOW() - INTERVAL '1 hour';
```

### 2. Debugging Tools

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
)

// Debug task handler that logs detailed information
func debugTaskHandler(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
    log.Printf("=== Debug Task Handler ===")
    log.Printf("Job ID: %d", helpers.Job.ID)
    log.Printf("Task: %s", helpers.Job.TaskIdentifier)
    log.Printf("Attempt: %d/%d", helpers.Job.Attempts, helpers.Job.MaxAttempts)
    log.Printf("Worker ID: %s", helpers.WorkerID)
    log.Printf("Payload: %s", string(payload))

    // Check database connectivity
    if err := helpers.Pool.Ping(ctx); err != nil {
        log.Printf("Database error: %v", err)
        return err
    }

    // Process with detailed logging
    helpers.Logger.Info("Task processing started", map[string]interface{}{
        "job_id": helpers.Job.ID,
        "task":   helpers.Job.TaskIdentifier,
    })

    // Simulate work
    time.Sleep(1 * time.Second)

    helpers.Logger.Info("Task processing completed", map[string]interface{}{
        "job_id": helpers.Job.ID,
    })

    return nil
}
```

### 3. Performance Monitoring

```bash
#!/bin/bash
# monitoring.sh - Basic monitoring script

echo "=== KongTask Health Check ==="
curl -s http://localhost:8080/health | jq .

echo -e "\n=== Database Statistics ==="
psql $DATABASE_URL -c "
SELECT
    schemaname,
    tablename,
    n_tup_ins as inserts,
    n_tup_upd as updates,
    n_tup_del as deletes
FROM pg_stat_user_tables
WHERE schemaname = 'graphile_worker';
"

echo -e "\n=== Active Jobs ==="
psql $DATABASE_URL -c "
SELECT
    task_identifier,
    count(*) as active_count
FROM graphile_worker.jobs
WHERE locked_at IS NOT NULL
GROUP BY task_identifier;
"

echo -e "\n=== Queue Status ==="
psql $DATABASE_URL -c "
SELECT
    queue_name,
    count(*) as pending_jobs
FROM graphile_worker.jobs
WHERE locked_at IS NULL
GROUP BY queue_name;
"
```

This deployment guide covers the essential aspects of running KongTask in production. For specific deployment scenarios or advanced configurations, refer to the [API Reference](API_REFERENCE.md) and other documentation files.
