# Enhanced Error Handling Example

This example demonstrates the enhanced error handling and panic recovery features implemented in kongtask, synchronized from graphile-worker commit 79f2160.

## Features Demonstrated

### 1. Panic Recovery

- Worker goroutines can panic without bringing down the entire worker pool
- Panics are automatically recovered and converted to error events
- The worker pool continues operating even after individual worker panics

### 2. Critical Error Handling

- Critical errors (like database connection failures) trigger graceful shutdown
- Non-critical errors are logged but don't stop the pool
- Error classification helps distinguish between recoverable and fatal errors

### 3. Event-Driven Error Monitoring

- Pool error events are emitted for monitoring and alerting
- Graceful shutdown events provide visibility into pool lifecycle
- Centralized error aggregation prevents unhandled error scenarios

### 4. Enhanced Wait API

- `Wait()` method now returns critical errors that caused shutdown
- `WaitWithoutError()` maintains backward compatibility
- Better error visibility for application-level error handling

## Running the Example

1. Set up PostgreSQL with the graphile_worker schema
2. Update the connection string in the code
3. Run the example:

```bash
go run examples/error_handling/main.go
```

## Expected Behavior

The example will:

1. **Process safe tasks normally** - These complete without issues
2. **Recover from panic** - The risky task with `panic: true` will panic, but the worker pool will recover and continue
3. **Handle critical errors** - The critical error task will trigger a graceful shutdown
4. **Report final status** - The Wait() method will return the critical error that caused shutdown

## Key Implementation Details

### Error Channel

Each WorkerPool has an error channel for aggregating critical errors from all goroutines:

```go
type WorkerPool struct {
    // ...
    errorChan     chan error
    criticalError error
}
```

### Panic Recovery Pattern

All goroutines use a consistent panic recovery pattern:

```go
defer func() {
    if r := recover(); r != nil {
        panicErr := fmt.Errorf("panic: %v", r)
        select {
        case wp.errorChan <- panicErr:
        default:
            // Handle full error channel
        }
    }
}()
```

### Error Classification

Errors are classified to determine shutdown behavior:

```go
func (wp *WorkerPool) isCriticalError(err error) bool {
    // Connection errors are critical
    // Context cancellation is not critical
    // Application errors are typically not critical
}
```

## Alignment with graphile-worker

This implementation mirrors the error handling improvements in graphile-worker commit 79f2160:

- **Unified error handling** prevents unhandled Promise rejections (Go: panics)
- **Centralized error aggregation** combines errors from multiple subsystems
- **Graceful degradation** allows the system to continue operating after recoverable errors
- **Enhanced observability** through comprehensive event emission

The Go implementation adapts these concepts to Go's error handling patterns and goroutine model while maintaining the same robustness and reliability improvements.
