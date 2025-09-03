package env

import (
	"os"
	"strconv"
	"strings"
)

// Environment variable handling utilities for KongTask
// Provides centralized environment variable management with consistent naming and fallbacks

// GetStringWithFallback returns the value of the first non-empty environment variable
// or the default value if all are empty
func GetStringWithFallback(defaultValue string, envVars ...string) string {
	for _, envVar := range envVars {
		if value := os.Getenv(envVar); value != "" {
			return value
		}
	}
	return defaultValue
}

// GetIntWithFallback returns the integer value of the first parseable environment variable
// or the default value if all are empty or unparseable
func GetIntWithFallback(defaultValue int, envVars ...string) int {
	for _, envVar := range envVars {
		if value := os.Getenv(envVar); value != "" {
			if intValue, err := strconv.Atoi(value); err == nil {
				return intValue
			}
		}
	}
	return defaultValue
}

// GetBoolWithFallback returns the boolean value of the first parseable environment variable
// or the default value if all are empty or unparseable
// Accepts: "true", "1", "yes", "on" (case insensitive) as true
func GetBoolWithFallback(defaultValue bool, envVars ...string) bool {
	for _, envVar := range envVars {
		if value := os.Getenv(envVar); value != "" {
			switch strings.ToLower(value) {
			case "true", "1", "yes", "on":
				return true
			case "false", "0", "no", "off":
				return false
			}
		}
	}
	return defaultValue
}

// Schema returns the schema name from environment variables with proper fallback
// Priority: GRAPHILE_WORKER_SCHEMA > default
func Schema(defaultSchema string) string {
	return GetStringWithFallback(defaultSchema, "GRAPHILE_WORKER_SCHEMA")
}

// DatabaseURL returns the database URL from environment variables with proper fallback
// Priority: DATABASE_URL > constructed from PG* variables
func DatabaseURL() string {
	return GetStringWithFallback("", "DATABASE_URL")
}

// IsDebugEnabled checks if debug logging is enabled
// Priority: KONGTASK_DEBUG > GRAPHILE_WORKER_DEBUG
func IsDebugEnabled() bool {
	return GetBoolWithFallback(false, "KONGTASK_DEBUG", "GRAPHILE_WORKER_DEBUG")
}

// LogLevel returns the configured log level
// Priority: KONGTASK_LOG_LEVEL > GRAPHILE_WORKER_LOG_LEVEL > "info"
func LogLevel() string {
	return strings.ToLower(GetStringWithFallback("info", "KONGTASK_LOG_LEVEL", "GRAPHILE_WORKER_LOG_LEVEL"))
}

// HasPGVariables checks if PostgreSQL environment variables are available
func HasPGVariables() bool {
	return os.Getenv("PGDATABASE") != ""
}

// Concurrency returns the configured concurrency level
// Priority: KONGTASK_CONCURRENCY > GRAPHILE_WORKER_CONCURRENCY
func Concurrency(defaultConcurrency int) int {
	return GetIntWithFallback(defaultConcurrency, "KONGTASK_CONCURRENCY", "GRAPHILE_WORKER_CONCURRENCY")
}

// MaxPoolSize returns the configured maximum pool size
// Priority: KONGTASK_MAX_POOL_SIZE > GRAPHILE_WORKER_MAX_POOL_SIZE
func MaxPoolSize(defaultSize int) int {
	return GetIntWithFallback(defaultSize, "KONGTASK_MAX_POOL_SIZE", "GRAPHILE_WORKER_MAX_POOL_SIZE")
}
