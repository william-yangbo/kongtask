package logger

import (
	"fmt"
	"os"
	"strings"

	"github.com/william-yangbo/kongtask/pkg/env"
)

// LogScope represents the scope/context information for logging
type LogScope struct {
	Label          string  `json:"label,omitempty"`
	WorkerID       string  `json:"worker_id,omitempty"`
	TaskIdentifier string  `json:"task_identifier,omitempty"`
	JobID          *string `json:"job_id,omitempty"` // Changed to string in v0.4.0
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
		// Skip debug messages unless explicitly enabled (mirrors TypeScript GRAPHILE_WORKER_DEBUG)
		if level == LogLevelDebug && !env.IsDebugEnabled() {
			return
		}

		// Build scope string exactly like TypeScript
		scopeStr := scope.Label
		if scopeStr == "" {
			scopeStr = "core"
		}

		// Build worker part exactly like TypeScript
		workerPart := ""
		if scope.WorkerID != "" {
			workerPart = fmt.Sprintf("(%s", scope.WorkerID)
			if scope.TaskIdentifier != "" {
				workerPart += fmt.Sprintf(": %s", scope.TaskIdentifier)
			}
			if scope.JobID != nil {
				workerPart += fmt.Sprintf("{%s}", *scope.JobID)
			}
			workerPart += ")"
		}

		// Determine log method (match TypeScript console methods)
		var logMethod func(string, ...interface{})
		switch level {
		case LogLevelError:
			logMethod = func(format string, args ...interface{}) {
				fmt.Fprintf(os.Stderr, format+"\n", args...)
			}
		case LogLevelWarning:
			logMethod = func(format string, args ...interface{}) {
				fmt.Fprintf(os.Stderr, format+"\n", args...)
			}
		case LogLevelInfo:
			logMethod = func(format string, args ...interface{}) {
				_, _ = fmt.Fprintf(os.Stdout, format+"\n", args...)
			}
		default:
			logMethod = func(format string, args ...interface{}) {
				_, _ = fmt.Fprintf(os.Stdout, format+"\n", args...)
			}
		}

		// Format message exactly like TypeScript: [%s%s] %s: %s (with uppercase level)
		levelStr := strings.ToUpper(string(level))
		logMethod("[%s%s] %s: %s", scopeStr, workerPart, levelStr, message)

		// Log metadata if present (TypeScript version doesn't output metadata by default)
		if len(meta) > 0 && len(meta[0]) > 0 {
			logMethod("[%s%s] %s: metadata: %+v", scopeStr, workerPart, levelStr, meta[0])
		}
	}
}

// DefaultLogger provides a default logger instance
var DefaultLogger = NewLogger(ConsoleLogFactory)
