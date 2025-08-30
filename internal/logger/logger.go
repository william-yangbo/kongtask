package logger

import (
	"fmt"
	"log"
	"os"
)

// LogScope represents the scope/context information for logging
type LogScope struct {
	Label          string `json:"label,omitempty"`
	WorkerID       string `json:"worker_id,omitempty"`
	TaskIdentifier string `json:"task_identifier,omitempty"`
	JobID          *int   `json:"job_id,omitempty"`
}

// LogMeta represents additional metadata for structured logging
type LogMeta map[string]interface{}

// LogLevel represents the severity level of a log message
type LogLevel string

const (
	LogLevelError   LogLevel = "error"
	LogLevelWarning LogLevel = "warning"
	LogLevelInfo    LogLevel = "info"
	LogLevelDebug   LogLevel = "debug"
)

// LogFunction is the function signature for logging
type LogFunction func(level LogLevel, message string, meta ...LogMeta)

// LogFunctionFactory creates a LogFunction with the given scope
type LogFunctionFactory func(scope LogScope) LogFunction

// Logger provides structured logging capabilities
type Logger struct {
	scope      LogScope
	logFactory LogFunctionFactory
	logFn      LogFunction
}

// NewLogger creates a new Logger instance
func NewLogger(logFactory LogFunctionFactory, scope ...LogScope) *Logger {
	var finalScope LogScope
	if len(scope) > 0 {
		finalScope = scope[0]
	}

	return &Logger{
		scope:      finalScope,
		logFactory: logFactory,
		logFn:      logFactory(finalScope),
	}
}

// Scope returns a new Logger with additional scope information
func (l *Logger) Scope(additionalScope LogScope) *Logger {
	newScope := l.scope

	// Merge scopes, with additionalScope taking precedence
	if additionalScope.Label != "" {
		newScope.Label = additionalScope.Label
	}
	if additionalScope.WorkerID != "" {
		newScope.WorkerID = additionalScope.WorkerID
	}
	if additionalScope.TaskIdentifier != "" {
		newScope.TaskIdentifier = additionalScope.TaskIdentifier
	}
	if additionalScope.JobID != nil {
		newScope.JobID = additionalScope.JobID
	}

	return &Logger{
		scope:      newScope,
		logFactory: l.logFactory,
		logFn:      l.logFactory(newScope),
	}
}

// Error logs an error message
func (l *Logger) Error(message string, meta ...LogMeta) {
	l.logFn(LogLevelError, message, meta...)
}

// Warn logs a warning message
func (l *Logger) Warn(message string, meta ...LogMeta) {
	l.logFn(LogLevelWarning, message, meta...)
}

// Info logs an informational message
func (l *Logger) Info(message string, meta ...LogMeta) {
	l.logFn(LogLevelInfo, message, meta...)
}

// Debug logs a debug message
func (l *Logger) Debug(message string, meta ...LogMeta) {
	l.logFn(LogLevelDebug, message, meta...)
}

// ConsoleLogFactory is the default console-based log factory
func ConsoleLogFactory(scope LogScope) LogFunction {
	return func(level LogLevel, message string, meta ...LogMeta) {
		// Skip debug messages unless explicitly enabled
		if level == LogLevelDebug && os.Getenv("KONGTASK_DEBUG") == "" {
			return
		}

		// Build scope string
		scopeStr := scope.Label
		if scopeStr == "" {
			scopeStr = "core"
		}

		if scope.WorkerID != "" {
			scopeStr += fmt.Sprintf("(%s", scope.WorkerID)
			if scope.TaskIdentifier != "" {
				scopeStr += fmt.Sprintf(": %s", scope.TaskIdentifier)
			}
			if scope.JobID != nil {
				scopeStr += fmt.Sprintf("{%d}", *scope.JobID)
			}
			scopeStr += ")"
		}

		// Determine log method
		var logMethod func(string, ...interface{})
		switch level {
		case LogLevelError:
			logMethod = log.Printf
		case LogLevelWarning:
			logMethod = log.Printf
		case LogLevelInfo:
			logMethod = log.Printf
		default:
			logMethod = log.Printf
		}

		// Format message
		prefix := fmt.Sprintf("[%s] %s", scopeStr, string(level))
		logMethod("%s: %s", prefix, message)

		// Log metadata if present
		if len(meta) > 0 && len(meta[0]) > 0 {
			logMethod("%s: metadata: %+v", prefix, meta[0])
		}
	}
}

// DefaultLogger provides a default logger instance
var DefaultLogger = NewLogger(ConsoleLogFactory)
