# Crontab (Recurring Tasks)

KongTask supports triggering recurring tasks according to a cron-like schedule. This is designed for recurring tasks such as sending weekly emails, running database maintenance tasks daily, performing data roll-ups hourly, downloading external data every 20 minutes, etc.

## Features

- **Guaranteed Execution**: Thanks to ACID-compliant transactions, no duplicate task schedules will occur
- **Backfill Support**: Can backfill missed jobs if desired (e.g. if the Worker wasn't running when the job was due to be scheduled)
- **Regular Job Queue**: Schedules tasks using KongTask's regular job queue, so you get all the regular features such as exponential back-off on failure
- **UTC Only**: All timestamps are handled in UTC for consistency

> **Note**: It is not intended that you add recurring tasks for each of your individual application users. Instead, you should have relatively few recurring tasks, and those tasks can create additional jobs for individual users (or process multiple users) if necessary.

## Crontab Syntax

Tasks are read from a `crontab` file or string. The syntax is not 100% compatible with traditional cron, but follows a similar pattern:

```crontab
# ┌───────────── UTC minute (0 - 59)
# │ ┌───────────── UTC hour (0 - 23)
# │ │ ┌───────────── UTC day of the month (1 - 31)
# │ │ │ ┌───────────── UTC month (1 - 12)
# │ │ │ │ ┌───────────── UTC day of the week (0 - 6) (Sunday to Saturday)
# │ │ │ │ │ ┌───────────── task (identifier) to schedule
# │ │ │ │ │ │    ┌────────── optional scheduling options
# │ │ │ │ │ │    │     ┌────── optional payload to merge
# │ │ │ │ │ │    │     │
# │ │ │ │ │ │    │     │
# * * * * * task ?opts {payload}
```

### Time Expression Support

For the first 5 fields we support:

- **Explicit numeric value**: `5` (exact minute/hour/etc)
- **Wildcard**: `*` (all valid values)
- **Step values**: `*/n` (all valid values divisible by n, e.g. `*/15` = every 15 minutes)
- **Range syntax**: `1-5` (values from 1 to 5 inclusive)
- **List syntax**: `1,3,5` (specific values)
- **Combinations**: `*/10,7,56-59` (step values combined with specific values and ranges)

### Task Identifier

The task identifier must:

- Start with a letter or underscore
- Contain only letters, numbers, underscores, hyphens, and colons
- Match an existing task handler in your application

### Options

Options are specified with a `?` prefix and use URL query parameter syntax:

- `id=custom_identifier` - Custom identifier for the scheduled job (defaults to task name)
- `fill=4w3d2h1m` - Backfill period (how far back to schedule missed jobs)
- `max=25` - Maximum attempts for this job (overrides default)
- `queue=queue_name` - Queue name for this job (runs in series with other jobs in same queue)
- `priority=5` - Job priority (lower numbers run first)

#### Backfill Period Format

The `fill` option accepts time period expressions:

- `s` - seconds
- `m` - minutes
- `h` - hours
- `d` - days
- `w` - weeks

Examples:

- `fill=1h` - backfill jobs missed in the last hour
- `fill=2d` - backfill jobs missed in the last 2 days
- `fill=4w3d2h1m` - backfill jobs missed in the last 4 weeks, 3 days, 2 hours, and 1 minute

### Payload

JSON payloads are specified in curly braces `{}` and will be merged with the job when it runs.

## Examples

### Basic Examples

```crontab
# Run every minute
* * * * * health_check

# Run daily at 4 AM UTC
0 4 * * * daily_backup

# Run every Sunday at 4 AM UTC
0 4 * * 0 weekly_report

# Run every Tuesday at 4 AM UTC with payload
0 4 * * 2 tuesday_task {"isTuesday": true}
```

### Advanced Examples

```crontab
# Complex schedule: every 10 minutes, plus at 7, and 56-59 minutes past the hour
# on January 1st at 1 AM on Mondays, with options and payload
*/10,7,56-59 1 1 1 1 complex_task ?id=custom_id&fill=4w3d2h1m&max=3&queue=priority&priority=1 {"config": {"retries": 5}}

# Hourly task with 2-hour backfill
0 * * * * hourly_sync ?fill=2h

# Daily task with custom queue
0 6 * * * daily_report ?queue=reports

# Weekly maintenance with custom identifier
0 2 * * 0 maintenance ?id=weekly_maintenance&max=5
```

### Comments and Formatting

```crontab
# This is a comment - lines starting with # are ignored

# Empty lines are also ignored

# Whitespace is flexible - these are equivalent:
* * * * * task_name
*     *      *       *       *      task_name

# But the task identifier cannot contain spaces
* * * * * valid_task_name
# * * * * * invalid task name  # This would cause an error
```

## Configuration

### Using Crontab Files

By default, KongTask looks for a `crontab` file in the current directory:

```go
// Default: reads from "./crontab" file
cronItems, err := getCronItems("./crontab", false)
if err != nil {
    log.Fatal(err)
}

workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
    CronItems: cronItems,
})
```

#### Custom Crontab Path

You can specify a custom crontab file path using the `--crontab` CLI option:

```bash
# Use a custom crontab file
kongtask --crontab /path/to/my/custom.crontab

# Use a different file name
kongtask --crontab ./schedules.txt
```

The system will log the status of cron functionality:

- If file exists: `Found crontab file 'path'; cron is enabled`
- If file missing: `Failed to read crontab file 'path'; cron is disabled`

### Using Crontab Strings

You can also provide crontab content directly as a string:

```go
crontabContent := `
# Daily backup at 2 AM
0 2 * * * backup_database

# Hourly cleanup
0 * * * * cleanup_temp_files ?fill=1h
`

parser := cron.NewParser()
cronItems, err := parser.ParseCrontab(crontabContent)
if err != nil {
    log.Fatal(err)
}

workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
    CronItems: cronItems,
})
```

### Programmatic Cron Items

For more complex scenarios, you can create cron items programmatically:

```go
import "github.com/william-yangbo/kongtask/pkg/cron"

cronItem := cron.CronItem{
    Task:       "daily_report",
    Identifier: "daily_report",
    Pattern:    "0 6 * * *", // 6 AM daily
    Options: cron.CronItemOptions{
        BackfillPeriod: 2 * time.Hour,
        QueueName:     &[]string{"reports"}[0],
        Priority:      &[]int{5}[0],
        MaxAttempts:   &[]int{10}[0],
    },
    Payload: map[string]interface{}{
        "reportType": "daily",
        "format":     "pdf",
    },
}

parser := cron.NewParser()
cronItems, err := parser.ParseCronItems([]cron.CronItem{cronItem})
if err != nil {
    log.Fatal(err)
}
```

## Error Handling

Common crontab parsing errors and their solutions:

### Invalid Syntax

```crontab
# Error: Too few parameters
* * * * missing_task

# Error: Invalid time values
60 * * * * invalid_minute  # Minutes must be 0-59

# Error: Invalid task name
* * * * * invalid task name  # No spaces in task names
```

### Invalid Options

```crontab
# Error: Unknown option
* * * * * task ?unknown=value

# Error: Invalid time period
* * * * * task ?fill=invalid
```

### Invalid JSON

```crontab
# Error: Invalid JSON syntax
* * * * * task {invalid: json}

# Correct JSON syntax
* * * * * task {"valid": "json"}
```

## Best Practices

### 1. Use Descriptive Task Names

```crontab
# Good
0 2 * * * backup_user_data
0 6 * * 1 generate_weekly_report

# Less clear
0 2 * * * task1
0 6 * * 1 job2
```

### 2. Set Appropriate Backfill Periods

```crontab
# For critical tasks, use longer backfill
0 0 * * * critical_daily_backup ?fill=1d

# For frequent tasks, use shorter backfill
*/5 * * * * frequent_sync ?fill=10m

# For optional tasks, no backfill
0 12 * * * optional_newsletter
```

### 3. Use Queues for Related Tasks

```crontab
# Group related tasks in the same queue for sequential execution
0 1 * * * db_maintenance_step1 ?queue=maintenance
0 1 * * * db_maintenance_step2 ?queue=maintenance
0 1 * * * db_maintenance_step3 ?queue=maintenance
```

### 4. Set Appropriate Priorities

```crontab
# Critical system tasks get higher priority (lower numbers)
0 * * * * system_health_check ?priority=1

# Regular tasks use default priority
0 6 * * * daily_report

# Background tasks get lower priority (higher numbers)
0 2 * * * cleanup_old_files ?priority=10
```

### 5. Handle Time Zones Properly

Since KongTask uses UTC, convert your local times:

```crontab
# If you want 6 AM EST (UTC-5), use 11 AM UTC
0 11 * * * morning_report

# If you want 6 AM PST (UTC-8), use 2 PM UTC
0 14 * * * west_coast_report
```

## Monitoring and Debugging

### Check Scheduled Jobs

You can query the database to see scheduled cron jobs:

```sql
-- See all known crontab entries
SELECT * FROM graphile_worker.known_crontabs;

-- See scheduled jobs from cron
SELECT * FROM graphile_worker.jobs
WHERE task_identifier LIKE 'cron:%';
```

### Enable Debug Logging

```go
workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
    Logger: logger.New(logger.DebugLevel), // Enable debug logging
})
```

Debug logs will show:

- Cron parsing results
- Job scheduling decisions
- Backfill operations
- Timing calculations

## Migration from Traditional Cron

If you're migrating from traditional cron, here are the key differences:

| Traditional Cron   | KongTask Crontab              |
| ------------------ | ----------------------------- |
| System cron daemon | Application-level scheduling  |
| Shell commands     | Go task handlers              |
| Local timezone     | UTC only                      |
| No built-in retry  | Automatic retry with backoff  |
| No job persistence | PostgreSQL-backed persistence |
| Limited monitoring | Full job queue monitoring     |

### Migration Example

Traditional cron:

```bash
# Run backup script daily at 2 AM
0 2 * * * /usr/local/bin/backup.sh
```

KongTask equivalent:

```crontab
# Run backup task daily at 2 AM UTC
0 2 * * * backup_database ?fill=1h&max=5
```

With corresponding Go task handler:

```go
tasks := map[string]worker.TaskHandler{
    "backup_database": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
        // Your backup logic here
        return runDatabaseBackup(ctx)
    },
}
```

This provides better error handling, monitoring, and integration with your application.
