# API Reference

## Worker Pool

### NewPool

```go
func NewPool(db *pgxpool.Pool, options *Options) (*Pool, error)
```

Creates a new worker pool with the given database connection and options.

### Options

```go
type Options struct {
    Concurrency  int           // Number of concurrent workers (default: 1)
    PollInterval int           // Poll interval in milliseconds (default: 2000)
    Logger       Logger        // Custom logger (optional)
    Schema       string        // Database schema (default: "graphile_worker")
}
```

### Pool Methods

#### RegisterTask

```go
func (p *Pool) RegisterTask(name string, handler TaskHandler) error
```

Register a task handler for the given task name.

#### AddJob

```go
func (p *Pool) AddJob(ctx context.Context, taskName string, payload interface{}) error
```

Add a job to the queue with default options.

#### AddJobWithOptions

```go
func (p *Pool) AddJobWithOptions(ctx context.Context, taskName string, payload interface{}, options *JobOptions) error
```

Add a job with custom options:

```go
type JobOptions struct {
    QueueName   string    // Named queue for serialized execution
    RunAt       time.Time // Schedule job for later execution
    MaxAttempts int       // Custom retry limit
}
```

#### Start

```go
func (p *Pool) Start(ctx context.Context) error
```

Start the worker pool (blocks until context is cancelled).

#### Shutdown

```go
func (p *Pool) Shutdown(ctx context.Context) error
```

Gracefully shutdown the worker pool.

## Job Structure

```go
type Job struct {
    ID             int64           // Unique job identifier
    TaskIdentifier string          // Task name
    Payload        json.RawMessage // Job payload data
    QueueName      string          // Queue name
    CreatedAt      time.Time       // Creation timestamp
    Attempts       int             // Number of attempts so far
}
```

## Task Handler

```go
type TaskHandler func(ctx context.Context, job *Job) error
```

Task handlers should:

- Return `nil` on success
- Return `error` on failure (job will be retried)
- Be idempotent when possible
- Complete all async work before returning

## SQL Functions

### add_job

```sql
SELECT graphile_worker.add_job(
    identifier,     -- task name (required)
    payload,        -- JSON payload (default: {})
    queue_name,     -- queue name (default: random)
    run_at,         -- schedule time (default: now)
    max_attempts    -- retry limit (default: 25)
);
```

Examples:

```sql
-- Basic job
SELECT graphile_worker.add_job('send_email', '{"to": "user@example.com"}');

-- Scheduled job
SELECT graphile_worker.add_job(
    'generate_report',
    '{"user_id": 123}',
    'reports',
    NOW() + INTERVAL '1 hour',
    10
);
```

### Database Triggers

Auto-schedule jobs from database changes:

```sql
CREATE FUNCTION notify_new_user() RETURNS trigger AS $$
BEGIN
    PERFORM graphile_worker.add_job('welcome_email',
        json_build_object('user_id', NEW.id));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER new_user_trigger
    AFTER INSERT ON users
    FOR EACH ROW EXECUTE FUNCTION notify_new_user();
```
