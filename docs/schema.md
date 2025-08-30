# Database Schema

KongTask uses the same database schema as graphile-worker for 100% compatibility.

## Schema Overview

The migration creates the `graphile_worker` schema with the following components:

### Tables

#### migrations

Tracks applied schema migrations:

```sql
CREATE TABLE graphile_worker.migrations (
    id serial PRIMARY KEY,
    ts timestamptz DEFAULT now() NOT NULL
);
```

#### job_queues

Queue configuration and statistics:

```sql
CREATE TABLE graphile_worker.job_queues (
    queue_name text PRIMARY KEY,
    job_count int DEFAULT 0 NOT NULL,
    locked_at timestamptz,
    locked_by text
);
```

#### jobs

Individual job records:

```sql
CREATE TABLE graphile_worker.jobs (
    id bigserial PRIMARY KEY,
    queue_name text,
    task_identifier text NOT NULL,
    payload json DEFAULT '{}'::json NOT NULL,
    priority int DEFAULT 0 NOT NULL,
    run_at timestamptz DEFAULT now() NOT NULL,
    attempts int DEFAULT 0 NOT NULL,
    max_attempts int DEFAULT 25 NOT NULL,
    last_error text,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL
);
```

### Functions

#### add_job

Schedule a new job:

```sql
CREATE FUNCTION graphile_worker.add_job(
    identifier text,
    payload json DEFAULT NULL,
    queue_name text DEFAULT NULL,
    run_at timestamptz DEFAULT NULL,
    max_attempts int DEFAULT NULL
) RETURNS graphile_worker.jobs
```

#### get_job

Claim and retrieve a job for processing:

```sql
CREATE FUNCTION graphile_worker.get_job(
    worker_id text,
    task_identifiers text[] DEFAULT NULL,
    job_expiry interval DEFAULT '4 hours'::interval
) RETURNS graphile_worker.jobs
```

#### complete_job

Mark job as completed:

```sql
CREATE FUNCTION graphile_worker.complete_job(
    worker_id text,
    job_id bigint
) RETURNS graphile_worker.jobs
```

#### fail_job

Mark job as failed:

```sql
CREATE FUNCTION graphile_worker.fail_job(
    worker_id text,
    job_id bigint,
    error_message text
) RETURNS graphile_worker.jobs
```

### Indexes

Optimized for high-performance job processing:

```sql
-- Job retrieval optimization
CREATE INDEX jobs_priority_run_at_id_idx
ON graphile_worker.jobs(priority DESC, run_at, id)
WHERE (attempts < max_attempts);

-- Queue-specific job lookup
CREATE INDEX jobs_queue_name_priority_run_at_id_idx
ON graphile_worker.jobs(queue_name, priority DESC, run_at, id)
WHERE (attempts < max_attempts);

-- Job cleanup and monitoring
CREATE INDEX jobs_created_at_idx ON graphile_worker.jobs(created_at);
```

### Triggers

#### LISTEN/NOTIFY for real-time processing

```sql
CREATE FUNCTION graphile_worker.tg__add_job() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('jobs:insert', NEW.id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER _500_gw_insert_job
AFTER INSERT ON graphile_worker.jobs
FOR EACH ROW EXECUTE FUNCTION graphile_worker.tg__add_job();
```

## Migration Process

KongTask automatically applies migrations when needed:

```bash
# Automatic migration
kongtask migrate -c "postgres://localhost/mydb"

# Check migration status
kongtask migrate --status
```

### Manual Migration

If you prefer manual control:

```sql
-- Check current schema version
SELECT * FROM graphile_worker.migrations ORDER BY id DESC LIMIT 1;

-- Apply specific migration
-- (Migration files are embedded in the binary)
```

## Schema Compatibility

### graphile-worker Compatibility

- ✅ 100% compatible with graphile-worker v0.16+
- ✅ Can process jobs created by graphile-worker
- ✅ Jobs created by KongTask work with graphile-worker
- ✅ Shared database schemas work seamlessly

### Version Support

- PostgreSQL 12+ required
- Automatic extension installation (`pgcrypto`, `uuid-ossp`)
- Compatible with PostGIS and other PostgreSQL extensions

## Performance Optimizations

### SKIP LOCKED

Uses `SELECT ... FOR UPDATE SKIP LOCKED` for efficient job claiming without contention.

### Connection Pooling

Optimized for PostgreSQL connection pooling with configurable pool sizes.

### Prepared Statements

Uses prepared statements for high-frequency operations.

### Batch Operations

Supports batch job insertion for improved throughput.

## Monitoring Queries

### Job Statistics

```sql
-- Job counts by status
SELECT
    task_identifier,
    COUNT(*) as total_jobs,
    COUNT(*) FILTER (WHERE attempts = 0) as pending,
    COUNT(*) FILTER (WHERE attempts > 0 AND attempts < max_attempts) as retrying,
    COUNT(*) FILTER (WHERE attempts >= max_attempts) as failed
FROM graphile_worker.jobs
GROUP BY task_identifier;
```

### Queue Status

```sql
-- Queue activity
SELECT
    queue_name,
    job_count,
    locked_at,
    locked_by
FROM graphile_worker.job_queues
ORDER BY job_count DESC;
```

### Performance Metrics

```sql
-- Average processing time by task
SELECT
    task_identifier,
    AVG(updated_at - created_at) as avg_processing_time,
    COUNT(*) as total_processed
FROM graphile_worker.jobs
WHERE attempts > 0
GROUP BY task_identifier;
```

## Cleanup

### Remove Old Jobs

```sql
-- Clean up completed jobs older than 7 days
DELETE FROM graphile_worker.jobs
WHERE updated_at < NOW() - INTERVAL '7 days'
AND attempts > 0;
```

### Complete Uninstall

```sql
-- Remove all KongTask data
DROP SCHEMA graphile_worker CASCADE;
```
