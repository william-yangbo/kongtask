# Changelog

## v0.5.0

Complete alignment with graphile-worker v0.5.0, bringing administrative functions,
custom schema support, and significant architecture improvements for production use.

### v0.5.0 improvements:

- **Administrative functions** for bulk job management (ideal for administrative UIs):
  - `CompleteJobs`: Mark jobs as completed and remove them from the queue
  - `PermanentlyFailJobs`: Mark jobs as permanently failed
  - `RescheduleJobs`: Update job scheduling properties (run time, priority, attempts)
- **Custom schema support**: Ability to rename `graphile_worker` schema via
  `GRAPHILE_WORKER_SCHEMA` environment variable
- **Signal handling control**: Added `NoHandleSignals` option to disable automatic
  signal handlers (useful when you need custom signal handling)
- **Configuration file support**: Added cosmiconfig-like configuration loading:
  - `.graphile-workerrc`, `.graphile-workerrc.json`, `.graphile-workerrc.yaml`
  - `graphile-worker.config.json`, `graphile-worker.config.yaml`
  - Supports directory traversal up to find config files
- **Improved security**: Use `crypto/rand` instead of time-based IDs for worker
  identification, eliminating collision risks
- **Enhanced error handling**: Comprehensive error checking and proper resource cleanup
- **Production-ready**: Full test coverage with 33 integration tests

### v0.5.0 API changes:

The ability to override the SQL schema and improved consistency means that most APIs
now accept options as configuration parameters:

**New API signatures** (Go-style):

- `RunTaskList(ctx, options, tasks, pool)` - options moved to second parameter
- `RunTaskListOnce(ctx, taskMap, pool, options)` - consistent options handling
- `NewMigrator(pool, schema)` - explicit schema parameter
- `MakeWorkerUtils(ctx, options)` - centralized configuration
- `RunTaskListWithSignalHandling(ctx, options, tasks, pool)` - signal handling version

**Administrative functions**:

```go
// Complete jobs (deletes them)
completedJobs, err := workerUtils.CompleteJobs(ctx, []string{"123", "456"})

// Permanently fail jobs
failedJobs, err := workerUtils.PermanentlyFailJobs(ctx, []string{"789"}, "Manual failure")

// Reschedule jobs
rescheduledJobs, err := workerUtils.RescheduleJobs(ctx, []string{"101"}, RescheduleOptions{
    RunAt:       &futureTime,
    Priority:    &newPriority,
    Attempts:    &resetAttempts,
    MaxAttempts: &newMaxAttempts,
})
```

**Configuration examples**:

```go
// Using environment variable
os.Setenv("GRAPHILE_WORKER_SCHEMA", "my_custom_schema")

// Using configuration file (.graphile-workerrc.yaml)
// schema: my_custom_schema
// pollInterval: 5000
// maxContiguousErrors: 15

// Disabling signal handling
options := WorkerPoolOptions{
    NoHandleSignals: true, // You handle signals yourself
    Schema:          "graphile_worker",
    Concurrency:     4,
}
```

### Breaking changes from previous kongtask versions:

- **Schema awareness**: All functions now require schema configuration
- **Options-first**: Major APIs moved options to first or second parameter for consistency
- **Worker ID generation**: Changed from timestamp-based to cryptographically secure IDs
- **Configuration priority**: Environment variables now take precedence over config files

### Database compatibility:

- **Full SQL schema alignment**: All migrations match graphile-worker v0.5.0 exactly
- **Administrative functions**: Implemented as PostgreSQL functions, callable from SQL or Go
- **Custom schema**: Complete support for non-default schema names
- **Performance optimization**: Prepared statements and connection pooling

### Testing:

- **33 integration tests**: Complete coverage of all graphile-worker functionality
- **Admin functions test suite**: Dedicated testing for bulk operations
- **Altschema compatibility**: Custom schema testing with environment variables
- **Performance benchmarks**: Latency and throughput testing
- **Signal handling tests**: Graceful shutdown and error scenarios

### Migration from earlier versions:

1. **Update API calls**: Add options parameter to RunTaskList calls
2. **Schema configuration**: Ensure schema is properly configured in options
3. **Signal handling**: Review signal handling if using custom implementation
4. **Configuration files**: Optionally migrate to config file approach

### Notes:

- All APIs maintain backward compatibility within v0.5.x series
- Performance improvements through better connection management
- Enhanced logging and debugging capabilities
- Production-tested under various load conditions

---

## Earlier Versions

### v0.4.0

Performance improvements and job management API enhancements.

### v0.2.0

Core job queue functionality with basic worker pool support.

### v0.1.0

Initial release with basic job scheduling and execution.
