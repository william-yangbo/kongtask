# Custom Logging

KongTask provides a flexible logging system that allows you to customize where and how log messages are output. You can integrate with popular Go logging libraries or implement your own custom logging solution.

## Default Logger

By default, KongTask uses a console logger that outputs to stdout/stderr:

```go
workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
    // Uses default console logger
    Concurrency: 4,
})
```

## Custom Logger Interface

To provide custom logging, implement the `Logger` interface:

```go
type Logger interface {
    Debug(message string, meta map[string]interface{})
    Info(message string, meta map[string]interface{})
    Warn(message string, meta map[string]interface{})
    Error(message string, meta map[string]interface{})
}
```

## Built-in Logger Implementations

### Console Logger

The default console logger with configurable levels:

```go
import "github.com/william-yangbo/kongtask/pkg/logger"

// Create console logger with specific level
consoleLogger := logger.NewConsole(logger.InfoLevel)

workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
    Logger: consoleLogger,
})
```

Available log levels:

- `logger.DebugLevel` - Most verbose, includes all messages
- `logger.InfoLevel` - Standard operational messages
- `logger.WarnLevel` - Warning messages only
- `logger.ErrorLevel` - Error messages only

### JSON Logger

For structured logging output:

```go
import "github.com/william-yangbo/kongtask/pkg/logger"

// Create JSON logger
jsonLogger := logger.NewJSON(logger.InfoLevel)

workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
    Logger: jsonLogger,
})
```

JSON output example:

```json
{"level":"info","message":"Worker started","worker_id":"worker-123","timestamp":"2025-09-03T10:30:00Z"}
{"level":"info","message":"Job completed","job_id":42,"task":"send_email","duration":"150ms","timestamp":"2025-09-03T10:30:01Z"}
```

### Silent Logger

For testing or when you want to suppress all logging:

```go
import "github.com/william-yangbo/kongtask/pkg/logger"

silentLogger := logger.NewSilent()

workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
    Logger: silentLogger,
})
```

## Integration with Popular Logging Libraries

### Logrus Integration

```go
package main

import (
    "github.com/sirupsen/logrus"
    "github.com/william-yangbo/kongtask/pkg/worker"
)

type LogrusAdapter struct {
    logger *logrus.Logger
}

func NewLogrusAdapter() *LogrusAdapter {
    logger := logrus.New()
    logger.SetFormatter(&logrus.JSONFormatter{})
    logger.SetLevel(logrus.InfoLevel)

    return &LogrusAdapter{logger: logger}
}

func (l *LogrusAdapter) Debug(message string, meta map[string]interface{}) {
    l.logger.WithFields(logrus.Fields(meta)).Debug(message)
}

func (l *LogrusAdapter) Info(message string, meta map[string]interface{}) {
    l.logger.WithFields(logrus.Fields(meta)).Info(message)
}

func (l *LogrusAdapter) Warn(message string, meta map[string]interface{}) {
    l.logger.WithFields(logrus.Fields(meta)).Warn(message)
}

func (l *LogrusAdapter) Error(message string, meta map[string]interface{}) {
    l.logger.WithFields(logrus.Fields(meta)).Error(message)
}

func main() {
    logrusLogger := NewLogrusAdapter()

    workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
        Logger: logrusLogger,
    })
}
```

### Zap Integration

```go
package main

import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
    "github.com/william-yangbo/kongtask/pkg/worker"
)

type ZapAdapter struct {
    logger *zap.Logger
}

func NewZapAdapter() *ZapAdapter {
    config := zap.NewProductionConfig()
    config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)

    logger, _ := config.Build()
    return &ZapAdapter{logger: logger}
}

func (z *ZapAdapter) Debug(message string, meta map[string]interface{}) {
    z.logger.Debug(message, z.mapToFields(meta)...)
}

func (z *ZapAdapter) Info(message string, meta map[string]interface{}) {
    z.logger.Info(message, z.mapToFields(meta)...)
}

func (z *ZapAdapter) Warn(message string, meta map[string]interface{}) {
    z.logger.Warn(message, z.mapToFields(meta)...)
}

func (z *ZapAdapter) Error(message string, meta map[string]interface{}) {
    z.logger.Error(message, z.mapToFields(meta)...)
}

func (z *ZapAdapter) mapToFields(meta map[string]interface{}) []zap.Field {
    fields := make([]zap.Field, 0, len(meta))
    for key, value := range meta {
        fields = append(fields, zap.Any(key, value))
    }
    return fields
}

func main() {
    zapLogger := NewZapAdapter()
    defer zapLogger.logger.Sync()

    workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
        Logger: zapLogger,
    })
}
```

### Zerolog Integration

```go
package main

import (
    "os"
    "github.com/rs/zerolog"
    "github.com/william-yangbo/kongtask/pkg/worker"
)

type ZerologAdapter struct {
    logger zerolog.Logger
}

func NewZerologAdapter() *ZerologAdapter {
    logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
    zerolog.SetGlobalLevel(zerolog.InfoLevel)

    return &ZerologAdapter{logger: logger}
}

func (z *ZerologAdapter) Debug(message string, meta map[string]interface{}) {
    event := z.logger.Debug()
    for key, value := range meta {
        event = event.Interface(key, value)
    }
    event.Msg(message)
}

func (z *ZerologAdapter) Info(message string, meta map[string]interface{}) {
    event := z.logger.Info()
    for key, value := range meta {
        event = event.Interface(key, value)
    }
    event.Msg(message)
}

func (z *ZerologAdapter) Warn(message string, meta map[string]interface{}) {
    event := z.logger.Warn()
    for key, value := range meta {
        event = event.Interface(key, value)
    }
    event.Msg(message)
}

func (z *ZerologAdapter) Error(message string, meta map[string]interface{}) {
    event := z.logger.Error()
    for key, value := range meta {
        event = event.Interface(key, value)
    }
    event.Msg(message)
}

func main() {
    zerologLogger := NewZerologAdapter()

    workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
        Logger: zerologLogger,
    })
}
```

## Log Contexts and Metadata

KongTask automatically includes relevant context information in log messages:

### Worker Context

```go
// Logged when worker operations occur
{
    "worker_id": "worker-7327280603017288",
    "schema": "graphile_worker",
    "concurrency": 4
}
```

### Job Context

```go
// Logged during job processing
{
    "job_id": 42,
    "task": "send_email",
    "worker_id": "worker-7327280603017288",
    "attempts": 1,
    "max_attempts": 25,
    "run_at": "2025-09-03T10:30:00Z"
}
```

### Error Context

```go
// Logged when errors occur
{
    "error": "failed to send email: SMTP connection failed",
    "job_id": 42,
    "task": "send_email",
    "worker_id": "worker-7327280603017288",
    "attempts": 3,
    "retry_at": "2025-09-03T10:35:00Z"
}
```

## Custom Logger with File Output

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"
    "time"
)

type FileLogger struct {
    file     *os.File
    minLevel LogLevel
}

type LogLevel int

const (
    DebugLevel LogLevel = iota
    InfoLevel
    WarnLevel
    ErrorLevel
)

type LogEntry struct {
    Timestamp string                 `json:"timestamp"`
    Level     string                 `json:"level"`
    Message   string                 `json:"message"`
    Meta      map[string]interface{} `json:"meta,omitempty"`
}

func NewFileLogger(filename string, minLevel LogLevel) (*FileLogger, error) {
    file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
    if err != nil {
        return nil, err
    }

    return &FileLogger{
        file:     file,
        minLevel: minLevel,
    }, nil
}

func (f *FileLogger) log(level LogLevel, levelName, message string, meta map[string]interface{}) {
    if level < f.minLevel {
        return
    }

    entry := LogEntry{
        Timestamp: time.Now().UTC().Format(time.RFC3339),
        Level:     levelName,
        Message:   message,
        Meta:      meta,
    }

    data, _ := json.Marshal(entry)
    fmt.Fprintf(f.file, "%s\n", data)
}

func (f *FileLogger) Debug(message string, meta map[string]interface{}) {
    f.log(DebugLevel, "debug", message, meta)
}

func (f *FileLogger) Info(message string, meta map[string]interface{}) {
    f.log(InfoLevel, "info", message, meta)
}

func (f *FileLogger) Warn(message string, meta map[string]interface{}) {
    f.log(WarnLevel, "warn", message, meta)
}

func (f *FileLogger) Error(message string, meta map[string]interface{}) {
    f.log(ErrorLevel, "error", message, meta)
}

func (f *FileLogger) Close() error {
    return f.file.Close()
}

func main() {
    fileLogger, err := NewFileLogger("kongtask.log", InfoLevel)
    if err != nil {
        log.Fatal(err)
    }
    defer fileLogger.Close()

    workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
        Logger: fileLogger,
    })
}
```

## Multiple Logger Outputs

You can send logs to multiple destinations:

```go
package main

import (
    "github.com/william-yangbo/kongtask/pkg/worker"
)

type MultiLogger struct {
    loggers []worker.Logger
}

func NewMultiLogger(loggers ...worker.Logger) *MultiLogger {
    return &MultiLogger{loggers: loggers}
}

func (m *MultiLogger) Debug(message string, meta map[string]interface{}) {
    for _, logger := range m.loggers {
        logger.Debug(message, meta)
    }
}

func (m *MultiLogger) Info(message string, meta map[string]interface{}) {
    for _, logger := range m.loggers {
        logger.Info(message, meta)
    }
}

func (m *MultiLogger) Warn(message string, meta map[string]interface{}) {
    for _, logger := range m.loggers {
        logger.Warn(message, meta)
    }
}

func (m *MultiLogger) Error(message string, meta map[string]interface{}) {
    for _, logger := range m.loggers {
        logger.Error(message, meta)
    }
}

func main() {
    consoleLogger := logger.NewConsole(logger.InfoLevel)
    fileLogger, _ := NewFileLogger("kongtask.log", logger.DebugLevel)

    multiLogger := NewMultiLogger(consoleLogger, fileLogger)

    workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
        Logger: multiLogger,
    })
}
```

## Filtering and Sampling

For high-volume environments, you might want to filter or sample logs:

```go
type FilteredLogger struct {
    underlying worker.Logger
    sampleRate float64 // 0.0 to 1.0
}

func NewFilteredLogger(underlying worker.Logger, sampleRate float64) *FilteredLogger {
    return &FilteredLogger{
        underlying: underlying,
        sampleRate: sampleRate,
    }
}

func (f *FilteredLogger) shouldLog() bool {
    return rand.Float64() < f.sampleRate
}

func (f *FilteredLogger) Debug(message string, meta map[string]interface{}) {
    if f.shouldLog() {
        f.underlying.Debug(message, meta)
    }
}

func (f *FilteredLogger) Info(message string, meta map[string]interface{}) {
    // Always log info and above
    f.underlying.Info(message, meta)
}

func (f *FilteredLogger) Warn(message string, meta map[string]interface{}) {
    f.underlying.Warn(message, meta)
}

func (f *FilteredLogger) Error(message string, meta map[string]interface{}) {
    f.underlying.Error(message, meta)
}
```

## Sensitive Data Redaction

For security, you might want to redact sensitive information:

```go
type RedactingLogger struct {
    underlying   worker.Logger
    sensitiveKeys []string
}

func NewRedactingLogger(underlying worker.Logger, sensitiveKeys []string) *RedactingLogger {
    return &RedactingLogger{
        underlying:   underlying,
        sensitiveKeys: sensitiveKeys,
    }
}

func (r *RedactingLogger) redactMeta(meta map[string]interface{}) map[string]interface{} {
    if meta == nil {
        return nil
    }

    redacted := make(map[string]interface{})
    for key, value := range meta {
        if r.isSensitive(key) {
            redacted[key] = "[REDACTED]"
        } else {
            redacted[key] = value
        }
    }
    return redacted
}

func (r *RedactingLogger) isSensitive(key string) bool {
    for _, sensitiveKey := range r.sensitiveKeys {
        if key == sensitiveKey {
            return true
        }
    }
    return false
}

func (r *RedactingLogger) Debug(message string, meta map[string]interface{}) {
    r.underlying.Debug(message, r.redactMeta(meta))
}

func (r *RedactingLogger) Info(message string, meta map[string]interface{}) {
    r.underlying.Info(message, r.redactMeta(meta))
}

func (r *RedactingLogger) Warn(message string, meta map[string]interface{}) {
    r.underlying.Warn(message, r.redactMeta(meta))
}

func (r *RedactingLogger) Error(message string, meta map[string]interface{}) {
    r.underlying.Error(message, r.redactMeta(meta))
}

func main() {
    baseLogger := logger.NewJSON(logger.InfoLevel)
    redactingLogger := NewRedactingLogger(baseLogger, []string{"password", "token", "secret"})

    workerPool, err := worker.RunTaskList(ctx, tasks, pool, worker.WorkerPoolOptions{
        Logger: redactingLogger,
    })
}
```

## Performance Considerations

### Async Logging

For high-throughput scenarios, consider implementing async logging:

```go
type AsyncLogger struct {
    underlying worker.Logger
    logChan    chan logMessage
    done       chan struct{}
}

type logMessage struct {
    level   string
    message string
    meta    map[string]interface{}
}

func NewAsyncLogger(underlying worker.Logger, bufferSize int) *AsyncLogger {
    al := &AsyncLogger{
        underlying: underlying,
        logChan:    make(chan logMessage, bufferSize),
        done:       make(chan struct{}),
    }

    go al.processLogs()
    return al
}

func (a *AsyncLogger) processLogs() {
    for {
        select {
        case msg := <-a.logChan:
            switch msg.level {
            case "debug":
                a.underlying.Debug(msg.message, msg.meta)
            case "info":
                a.underlying.Info(msg.message, msg.meta)
            case "warn":
                a.underlying.Warn(msg.message, msg.meta)
            case "error":
                a.underlying.Error(msg.message, msg.meta)
            }
        case <-a.done:
            return
        }
    }
}

func (a *AsyncLogger) Debug(message string, meta map[string]interface{}) {
    select {
    case a.logChan <- logMessage{"debug", message, meta}:
    default:
        // Channel full, drop message
    }
}

// Implement other methods similarly...

func (a *AsyncLogger) Close() {
    close(a.done)
}
```

## Best Practices

### 1. Use Appropriate Log Levels

```go
// Debug: Detailed information for debugging
helpers.Logger.Debug("Processing job step", map[string]interface{}{
    "step": "validation",
    "data": sensitiveData, // Only in debug
})

// Info: General operational messages
helpers.Logger.Info("Job completed successfully", map[string]interface{}{
    "job_id":   job.ID,
    "duration": duration,
})

// Warn: Something unexpected but not an error
helpers.Logger.Warn("Retrying failed operation", map[string]interface{}{
    "attempt": attempt,
    "error":   err.Error(),
})

// Error: Something went wrong
helpers.Logger.Error("Job failed permanently", map[string]interface{}{
    "job_id":      job.ID,
    "error":       err.Error(),
    "max_attempts": job.MaxAttempts,
})
```

### 2. Include Relevant Context

```go
// Good: Include job context
helpers.Logger.Info("Starting payment processing", map[string]interface{}{
    "job_id":     job.ID,
    "user_id":    userID,
    "amount":     amount,
    "currency":   currency,
    "payment_id": paymentID,
})

// Less useful: Generic message
helpers.Logger.Info("Processing payment")
```

### 3. Structure Your Metadata

```go
// Good: Structured metadata
helpers.Logger.Info("API call completed", map[string]interface{}{
    "api": map[string]interface{}{
        "url":         apiURL,
        "method":      "POST",
        "status_code": resp.StatusCode,
        "duration":    duration.Milliseconds(),
    },
    "job": map[string]interface{}{
        "id":   job.ID,
        "task": job.TaskIdentifier,
    },
})

// Less organized: Flat metadata
helpers.Logger.Info("API call completed", map[string]interface{}{
    "url":         apiURL,
    "method":      "POST",
    "status_code": resp.StatusCode,
    "duration":    duration.Milliseconds(),
    "job_id":      job.ID,
    "task":        job.TaskIdentifier,
})
```

By implementing custom logging, you can integrate KongTask seamlessly with your existing observability infrastructure and ensure that all job processing activities are properly monitored and audited.
