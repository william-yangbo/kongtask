# Architecture Design

KongTask is designed as a high-performance, PostgreSQL-native job queue system with a focus on simplicity, reliability, and compatibility with graphile-worker.

## System Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    KongTask Architecture                    │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐     │
│  │   Client    │    │   Client    │    │   Client    │     │
│  │Application 1│    │Application 2│    │Application N│     │
│  └─────────────┘    └─────────────┘    └─────────────┘     │
│         │                   │                   │          │
│         └───────────────────┼───────────────────┘          │
│                             │                              │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │                Job Queue Layer                          │ │
│  │                                                         │ │
│  │  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐ │ │
│  │  │   Worker    │    │   Worker    │    │   Worker    │ │ │
│  │  │   Pool 1    │    │   Pool 2    │    │   Pool N    │ │ │
│  │  └─────────────┘    └─────────────┘    └─────────────┘ │ │
│  └─────────────────────────────────────────────────────────┘ │
│                             │                              │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │                PostgreSQL Database                      │ │
│  │                                                         │ │
│  │  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐ │ │
│  │  │    Jobs     │    │Job Queues   │    │ Functions   │ │ │
│  │  │   Table     │    │   Table     │    │& Triggers   │ │ │
│  │  └─────────────┘    └─────────────┘    └─────────────┘ │ │
│  └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Worker Pool (`internal/worker/pool.go`)

**Purpose**: Manages multiple worker goroutines and coordinates job processing.

**Key Responsibilities**:

- Connection pool management
- Worker lifecycle management
- Job distribution and load balancing
- Graceful shutdown coordination

```go
type Pool struct {
    db          *pgxpool.Pool
    workers     []*Worker
    options     *Options
    taskMap     map[string]TaskHandler
    cancelFunc  context.CancelFunc
    shutdownCh  chan struct{}
}
```

**Design Principles**:

- **Concurrency Control**: Configurable worker count based on workload
- **Resource Management**: Efficient database connection usage
- **Fault Tolerance**: Isolated worker failures don't affect the pool

### 2. Individual Worker (`internal/worker/worker.go`)

**Purpose**: Single worker goroutine that processes jobs sequentially.

**Job Processing Flow**:

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│    Poll     │───▶│ Claim Job   │───▶│ Execute     │───▶│  Complete   │
│  Database   │    │(SKIP LOCKED)│    │ Handler     │    │or Mark Failed│
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
       ▲                                                       │
       └───────────────────────────────────────────────────────┘
```

**Key Features**:

- **LISTEN/NOTIFY**: Real-time job notifications
- **SKIP LOCKED**: Non-blocking job claiming
- **Error Isolation**: Individual job failures don't crash worker

### 3. Database Layer (`internal/migrate/`)

**Purpose**: Database schema management and SQL function definitions.

**Schema Components**:

#### Tables

- **`jobs`**: Core job storage with payload and metadata
- **`job_queues`**: Queue configuration and statistics
- **`migrations`**: Schema version tracking

#### Functions

- **`add_job()`**: Schedule new jobs with options
- **`get_job()`**: Claim jobs for processing (SKIP LOCKED)
- **`complete_job()`**: Mark jobs as successfully completed
- **`fail_job()`**: Mark jobs as failed with error details

#### Triggers

- **`tg__add_job()`**: NOTIFY workers of new jobs via LISTEN/NOTIFY

## Design Patterns

### 1. Producer-Consumer Pattern

```
Producers (Clients)          Queue (PostgreSQL)         Consumers (Workers)
┌─────────────────┐         ┌─────────────────┐        ┌─────────────────┐
│ Web App         │───add──▶│     jobs        │◀──get──│ Worker Pool 1   │
│ API Server      │  job    │    table        │  job   │ Worker Pool 2   │
│ Database Trigger│         │                 │        │ Worker Pool N   │
│ Cron Job        │         └─────────────────┘        └─────────────────┘
└─────────────────┘
```

**Benefits**:

- **Decoupling**: Producers and consumers operate independently
- **Scalability**: Multiple producers and consumers can operate simultaneously
- **Reliability**: Jobs persist in database until successfully processed

### 2. Event-Driven Architecture

```
Database Event                LISTEN/NOTIFY               Worker Response
┌─────────────────┐          ┌─────────────────┐        ┌─────────────────┐
│ INSERT INTO     │ ──────── │ pg_notify()     │──────▶ │ Immediate       │
│ jobs            │ trigger  │ 'jobs:insert'   │ wakeup │ Processing      │
└─────────────────┘          └─────────────────┘        └─────────────────┘
```

**Benefits**:

- **Real-time Processing**: No polling delays
- **Resource Efficiency**: Workers sleep when no jobs available
- **Low Latency**: Jobs processed immediately upon insertion

### 3. Retry with Exponential Backoff

```
Job Execution Timeline:
Attempt 1: [FAIL] ──2.7s──▶ Attempt 2: [FAIL] ──7.4s──▶ Attempt 3: [SUCCESS]

Exponential Formula: delay = exp(least(10, attempt)) seconds
```

**Design Goals**:

- **Transient Failure Recovery**: Temporary issues resolve automatically
- **System Protection**: Prevents overwhelming external services
- **Configurable Limits**: Max attempts and custom retry logic

## Data Flow Architecture

### Job Lifecycle

```
1. Job Creation
   ┌─────────────────────────────────────────────────────────┐
   │ Client App ──add_job()──▶ PostgreSQL ──trigger──▶ NOTIFY │
   └─────────────────────────────────────────────────────────┘

2. Job Processing
   ┌─────────────────────────────────────────────────────────┐
   │ Worker ──get_job()──▶ SKIP LOCKED ──▶ Task Handler      │
   └─────────────────────────────────────────────────────────┘

3. Job Completion
   ┌─────────────────────────────────────────────────────────┐
   │ Handler ──return──▶ complete_job() or fail_job()        │
   └─────────────────────────────────────────────────────────┘
```

### Concurrency Model

```
Single Worker Pool:
┌─────────────────────────────────────────────────────────────┐
│ Pool ──spawn──▶ Worker 1 ──┐                               │
│      ──spawn──▶ Worker 2 ──┼──▶ Shared DB Connection Pool  │
│      ──spawn──▶ Worker N ──┘                               │
└─────────────────────────────────────────────────────────────┘

Multiple Worker Pools (Horizontal Scaling):
┌─────────────┐  ┌─────────────┐  ┌─────────────┐
│   Pool 1    │  │   Pool 2    │  │   Pool N    │
│ (Machine A) │  │ (Machine B) │  │ (Machine C) │
└─────────────┘  └─────────────┘  └─────────────┘
       │                │                │
       └────────────────┼────────────────┘
                        │
              ┌─────────────────┐
              │   PostgreSQL    │
              │   (Shared)      │
              └─────────────────┘
```

## Performance Architecture

### Database Optimizations

**Indexing Strategy**:

```sql
-- Hot path: job retrieval
CREATE INDEX jobs_priority_run_at_id_idx
ON jobs(priority DESC, run_at, id)
WHERE (attempts < max_attempts);

-- Queue-specific processing
CREATE INDEX jobs_queue_name_priority_run_at_id_idx
ON jobs(queue_name, priority DESC, run_at, id)
WHERE (attempts < max_attempts);
```

**Query Optimization**:

- **SKIP LOCKED**: Prevents worker contention
- **Prepared Statements**: Reduces parsing overhead
- **Connection Pooling**: Efficient resource utilization

### Memory Management

```
Worker Memory Layout:
┌─────────────────────────────────────────────────────────────┐
│ Stack: Task Handler Execution (~1-2MB per worker)          │
│ Heap: Job Payload Data (varies by job size)                │
│ Shared: DB Connection Pool (pooled across workers)         │
└─────────────────────────────────────────────────────────────┘

Optimization Strategies:
- Small payload sizes (reference large data by ID)
- Streaming processing for large datasets
- Explicit garbage collection in long-running handlers
```

## Error Handling Architecture

### Failure Isolation

```
Error Boundary Layers:
┌─────────────────────────────────────────────────────────────┐
│ Pool Level:     Continue with remaining workers            │
│ Worker Level:   Restart worker, continue with other jobs   │
│ Job Level:      Retry with exponential backoff             │
│ Handler Level:  Custom error handling and recovery         │
└─────────────────────────────────────────────────────────────┘
```

### Error Types

**Transient Errors** (Retriable):

- Database connection timeouts
- External service unavailability
- Temporary resource constraints

**Permanent Errors** (Non-retriable):

- Invalid job payload format
- Authentication failures
- Business logic violations

## Security Architecture

### Database Security

**Principle of Least Privilege**:

```sql
-- Worker role (minimal permissions)
GRANT SELECT, UPDATE ON graphile_worker.jobs TO worker_role;
GRANT EXECUTE ON FUNCTION graphile_worker.get_job TO worker_role;
GRANT EXECUTE ON FUNCTION graphile_worker.complete_job TO worker_role;
GRANT EXECUTE ON FUNCTION graphile_worker.fail_job TO worker_role;

-- Application role (job creation)
GRANT INSERT ON graphile_worker.jobs TO app_role;
GRANT EXECUTE ON FUNCTION graphile_worker.add_job TO app_role;
```

### Payload Security

**Recommendations**:

- Validate payload schemas in handlers
- Use encrypted connections (SSL/TLS)
- Avoid sensitive data in payloads (use references)
- Implement audit logging for sensitive operations

## Deployment Architecture

### Single-Instance Deployment

```
┌─────────────────────────────────────────────────────────────┐
│                    Single Server                            │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐     │
│  │   Web App   │    │  KongTask   │    │ PostgreSQL  │     │
│  │   Server    │    │   Worker    │    │  Database   │     │
│  └─────────────┘    └─────────────┘    └─────────────┘     │
└─────────────────────────────────────────────────────────────┘
```

### Distributed Deployment

```
┌─────────────┐  ┌─────────────┐  ┌─────────────┐
│  Web App 1  │  │  Web App 2  │  │  Web App N  │
│ (Producer)  │  │ (Producer)  │  │ (Producer)  │
└─────────────┘  └─────────────┘  └─────────────┘
       │                │                │
       └────────────────┼────────────────┘
                        │
┌─────────────┐  ┌─────────────┐  ┌─────────────┐
│  Worker     │  │  Worker     │  │  Worker     │
│  Pool 1     │  │  Pool 2     │  │  Pool N     │
│(Consumer)   │  │(Consumer)   │  │(Consumer)   │
└─────────────┘  └─────────────┘  └─────────────┘
       │                │                │
       └────────────────┼────────────────┘
                        │
              ┌─────────────────┐
              │   PostgreSQL    │
              │   (Shared)      │
              └─────────────────┘
```

## Monitoring Architecture

### Metrics Collection Points

```
Application Metrics:
- Job processing rate (jobs/second)
- Job completion latency (milliseconds)
- Error rates by task type
- Worker pool utilization

Database Metrics:
- Active job count by queue
- Failed job count by error type
- Database connection pool usage
- Query performance statistics

System Metrics:
- CPU and memory usage
- Network I/O patterns
- Disk I/O for PostgreSQL
```

### Health Check Strategy

```go
type HealthCheck struct {
    DatabaseConnectivity bool
    WorkerPoolStatus     bool
    JobProcessingRate    float64
    ErrorRate           float64
}
```

## Compatibility Architecture

### graphile-worker Compatibility

**Schema Compatibility**:

- Identical table structures
- Matching function signatures
- Compatible migration patterns
- Same retry behavior

**Behavioral Compatibility**:

- Job processing semantics
- Error handling patterns
- Queue behavior
- Performance characteristics

This architecture ensures KongTask can serve as a drop-in replacement while delivering superior performance through Go's native concurrency model.

## Future Architecture Considerations

### Planned Enhancements

**Job Prioritization**:

- Priority-based job processing
- Queue-specific priority handling
- Dynamic priority adjustment

**Advanced Scheduling**:

- Cron-like recurring jobs
- Dependency-based job chains
- Conditional job execution

**Enhanced Monitoring**:

- Built-in metrics endpoints
- Distributed tracing support
- Real-time dashboard integration

**Horizontal Scaling**:

- Automatic worker scaling
- Load-based job distribution
- Regional job processing
