# API Reference

This document provides comprehensive API reference for KongTask, covering all major interfaces, types, and functions.

## Core Interfaces

### TaskHandler

The fundamental interface for defining job processing logic.

```go
type TaskHandler func(ctx context.Context, payload json.RawMessage, helpers *Helpers) error
```

**Parameters:**

- `ctx` - Context for cancellation and timeouts
- `payload` - JSON payload data for the job
- `helpers` - Helper utilities for logging, database access, etc.

**Returns:**

- `error` - Return nil for success, error for failure (will trigger retry)

### Logger

Interface for custom logging implementations.

```go
type Logger interface {
    Debug(message string, meta map[string]interface{})
    Info(message string, meta map[string]interface{})
    Warn(message string, meta map[string]interface{})
    Error(message string, meta map[string]interface{})
}
```

### EventBus

Interface for handling worker and job lifecycle events.

```go
type EventBus interface {
    On(event string, handler func(data interface{}))
    Emit(event string, data interface{})
}
```

## Main Functions

### RunTaskList

Run a worker pool with specified tasks.

```go
func RunTaskList(
    ctx context.Context,
    tasks map[string]TaskHandler,
    pool *pgxpool.Pool,
    options WorkerPoolOptions,
) (*WorkerPool, error)
```

**Parameters:**

- `ctx` - Context for cancellation
- `tasks` - Map of task name to handler function
- `pool` - PostgreSQL connection pool
- `options` - Configuration options

**Returns:**

- `*WorkerPool` - Worker pool instance
- `error` - Setup error if any

### RunTaskListOnce

Run tasks once until no more jobs are available.

```go
func RunTaskListOnce(
    ctx context.Context,
    tasks map[string]TaskHandler,
    pool *pgxpool.Pool,
    options WorkerPoolOptions,
) error
```

### RunMigrations

Run database schema migrations.

```go
func RunMigrations(
    ctx context.Context,
    pool *pgxpool.Pool,
    schema string,
) error
```

## Worker Types

### WorkerPoolOptions

Configuration for worker pool behavior.

```go
type WorkerPoolOptions struct {
    // Number of concurrent workers (default: 1)
    Concurrency int

    // Database schema name (default: "graphile_worker")
    Schema string

    // Polling interval for new jobs (default: 2s)
    PollInterval time.Duration

    // Maximum PostgreSQL pool size (default: 10)
    MaxPoolSize int

    // Custom logger implementation
    Logger Logger

    // Event bus for monitoring
    Events EventBus

    // Cron items for scheduled tasks
    CronItems []cron.ParsedCronItem

    // Whether to handle OS signals (default: true)
    NoHandleSignals bool

    // Forbidden flags (static)
    ForbiddenFlags []string

    // Forbidden flags function (dynamic)
    ForbiddenFlagsFn func() ([]string, error)
}
```

### WorkerPool

Represents a pool of workers processing jobs.

```go
type WorkerPool struct {
    // Wait for all workers to complete
    Wait() error

    // Release the worker pool
    Release() error

    // Add a job to the queue
    AddJob(ctx context.Context, task string, payload interface{}, spec *TaskSpec) error
}
```

### Worker

Individual worker for job operations.

```go
type Worker struct {
    // Add a job to the queue
    AddJob(ctx context.Context, task string, payload interface{}) error

    // Add job with custom options
    AddJobWithSpec(ctx context.Context, task string, payload interface{}, spec *TaskSpec) error

    // Convenience methods for JobKeyMode
    AddJobWithReplace(ctx context.Context, task string, payload interface{}, jobKey string) error
    AddJobWithPreserveRunAt(ctx context.Context, task string, payload interface{}, jobKey string) error
    AddJobWithUnsafeDedupe(ctx context.Context, task string, payload interface{}, jobKey string) error
}
```

### Helpers

Helper utilities available to task handlers.

```go
type Helpers struct {
    // Logger instance
    Logger Logger

    // Database connection pool
    Pool *pgxpool.Pool

    // Add another job from within a task
    AddJob func(ctx context.Context, task string, payload interface{}) error

    // Database schema name
    Schema string

    // Current job information
    Job *Job

    // Worker ID
    WorkerID string
}
```

## Job Specifications

### TaskSpec

Options for job scheduling and behavior.

```go
type TaskSpec struct {
    // Queue name for sequential processing
    QueueName *string

    // Scheduled execution time
    RunAt *time.Time

    // Job priority (lower = higher priority)
    Priority *int

    // Maximum retry attempts
    MaxAttempts *int

    // Unique job identifier
    JobKey *string

    // Job key behavior mode
    JobKeyMode *string // "replace", "preserve_run_at", "unsafe_dedupe"

    // Job flags for filtering
    Flags []string
}
```

### Job

Represents a job in the queue.

```go
type Job struct {
    ID             int             `json:"id"`
    QueueName      *string         `json:"queue_name"`
    TaskIdentifier string          `json:"task_identifier"`
    Payload        json.RawMessage `json:"payload"`
    Priority       int             `json:"priority"`
    RunAt          time.Time       `json:"run_at"`
    Attempts       int             `json:"attempts"`
    MaxAttempts    int             `json:"max_attempts"`
    LastError      *string         `json:"last_error"`
    CreatedAt      time.Time       `json:"created_at"`
    UpdatedAt      time.Time       `json:"updated_at"`
    Key            *string         `json:"key"`
    Revision       int             `json:"revision"`
    LockedAt       *time.Time      `json:"locked_at"`
    LockedBy       *string         `json:"locked_by"`
    Flags          []string        `json:"flags"`
}
```

## Administrative Functions

### WorkerUtils

Administrative operations on the job queue.

```go
type WorkerUtils struct {
    // Complete jobs by ID
    CompleteJobs(ctx context.Context, jobIDs []int) ([]Job, error)

    // Permanently fail jobs
    PermanentlyFailJobs(ctx context.Context, jobIDs []int, reason string) ([]Job, error)

    // Reschedule jobs with new parameters
    RescheduleJobs(ctx context.Context, jobIDs []int, spec RescheduleSpec) ([]Job, error)

    // Add job to queue
    AddJob(ctx context.Context, task string, payload interface{}, spec *TaskSpec) (*Job, error)
}
```

### RescheduleSpec

Options for rescheduling jobs.

```go
type RescheduleSpec struct {
    RunAt       *time.Time `json:"run_at,omitempty"`
    Priority    *int       `json:"priority,omitempty"`
    Attempts    *int       `json:"attempts,omitempty"`
    MaxAttempts *int       `json:"max_attempts,omitempty"`
}
```

## Cron Support

### CronItem

Configuration for a scheduled recurring task.

```go
type CronItem struct {
    Task       string                 `json:"task"`
    Identifier string                 `json:"identifier"`
    Pattern    string                 `json:"pattern"`
    Options    CronItemOptions        `json:"options"`
    Payload    map[string]interface{} `json:"payload"`
}
```

### CronItemOptions

Options for cron job behavior.

```go
type CronItemOptions struct {
    BackfillPeriod time.Duration `json:"backfill_period"`
    QueueName      *string       `json:"queue_name"`
    Priority       *int          `json:"priority"`
    MaxAttempts    *int          `json:"max_attempts"`
}
```

### ParsedCronItem

Internal representation of a parsed cron item.

```go
type ParsedCronItem struct {
    Job        Job               `json:"job"`
    Identifier string            `json:"identifier"`
    Minutes    []int             `json:"minutes"`
    Hours      []int             `json:"hours"`
    Dates      []int             `json:"dates"`
    Months     []int             `json:"months"`
    DOWs       []int             `json:"dows"`
    Options    CronItemOptions   `json:"options"`
}
```

## Events

### Event Types

KongTask emits the following events:

```go
const (
    // Worker events
    WorkerCreate     = "worker:create"
    WorkerRelease    = "worker:release"
    WorkerStop       = "worker:stop"
    WorkerGetJobStart = "worker:getJob:start"

    // Job events
    JobStart    = "job:start"
    JobComplete = "job:complete"
    JobSuccess  = "job:success"
    JobError    = "job:error"
    JobFailed   = "job:failed"

    // Pool events
    PoolCreate  = "pool:create"
    PoolRelease = "pool:release"
    Stop        = "stop"
)
```

### Event Data Structures

#### WorkerCreateData

```go
type WorkerCreateData struct {
    WorkerID string `json:"worker_id"`
}
```

#### JobStartData

```go
type JobStartData struct {
    WorkerID string `json:"worker_id"`
    Job      *Job   `json:"job"`
}
```

#### JobCompleteData

```go
type JobCompleteData struct {
    WorkerID string `json:"worker_id"`
    Job      *Job   `json:"job"`
    Error    error  `json:"error,omitempty"`
}
```

## Error Handling

### Error Codes

KongTask uses specific error codes for validation:

- `GWBID` - Task identifier too long (max: 128 chars)
- `GWBQN` - Queue name too long (max: 128 chars)
- `GWBJK` - Job key too long (max: 512 chars)
- `GWBMA` - Max attempts must be >= 1
- `GWBKM` - Invalid job_key_mode value

### Common Errors

```go
// Validation errors
var (
    ErrTaskIdentifierTooLong = errors.New("GWBID: task identifier too long")
    ErrQueueNameTooLong     = errors.New("GWBQN: queue name too long")
    ErrJobKeyTooLong        = errors.New("GWBJK: job key too long")
    ErrInvalidMaxAttempts   = errors.New("GWBMA: max attempts must be >= 1")
    ErrInvalidJobKeyMode    = errors.New("GWBKM: invalid job_key_mode")
)
```

## Usage Examples

### Basic Worker Setup

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
    // Database connection
    pool, err := pgxpool.New(context.Background(), "postgres://user:pass@localhost/db")
    if err != nil {
        log.Fatal(err)
    }
    defer pool.Close()

    // Task handlers
    tasks := map[string]worker.TaskHandler{
        "send_email": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
            var email struct {
                To      string `json:"to"`
                Subject string `json:"subject"`
            }

            if err := json.Unmarshal(payload, &email); err != nil {
                return err
            }

            helpers.Logger.Info("Sending email", map[string]interface{}{
                "to": email.To,
                "subject": email.Subject,
            })

            // Email sending logic here
            return nil
        },
    }

    // Start worker pool
    workerPool, err := worker.RunTaskList(context.Background(), tasks, pool, worker.WorkerPoolOptions{
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
        "subject": "Hello World",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Wait for completion
    workerPool.Wait()
}
```

### Advanced Job Scheduling

```go
// Schedule a job for future execution
spec := &worker.TaskSpec{
    RunAt:       &futureTime,
    Priority:    &[]int{5}[0],
    MaxAttempts: &[]int{10}[0],
    QueueName:   &[]string{"high-priority"}[0],
    JobKey:      &[]string{"unique-key-123"}[0],
    JobKeyMode:  &[]string{"replace"}[0],
    Flags:       []string{"urgent", "customer-facing"},
}

job, err := workerUtils.AddJob(ctx, "process_payment", paymentData, spec)
```

### Event Monitoring

```go
import "github.com/william-yangbo/kongtask/pkg/events"

eventBus := events.NewEventBus()

// Monitor job completion
eventBus.On(events.JobComplete, func(data interface{}) {
    if jobData, ok := data.(events.JobCompleteData); ok {
        log.Printf("Job %d completed by worker %s",
            jobData.Job.ID, jobData.WorkerID)
    }
})

// Use in worker configuration
workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
    Events: eventBus,
})
```

### Custom Logger Integration

```go
import "github.com/sirupsen/logrus"

type LogrusAdapter struct {
    logger *logrus.Logger
}

func (l *LogrusAdapter) Info(message string, meta map[string]interface{}) {
    l.logger.WithFields(logrus.Fields(meta)).Info(message)
}

// ... implement other methods

logrusLogger := &LogrusAdapter{logger: logrus.New()}

workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
    Logger: logrusLogger,
})
```

This API reference covers all the major interfaces and functions available in KongTask. For more detailed examples and use cases, see the specific documentation files for [Task Handlers](TASK_HANDLERS.md), [Crontab](CRONTAB.md), and [Logging](LOGGING.md).
