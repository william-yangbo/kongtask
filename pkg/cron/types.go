// Package cron provides cron scheduling functionality for kongtask
// This package implements distributed cron scheduling with backfill support,
// synchronized from graphile-worker commit cb369ad
package cron

import (
	"context"
	"fmt"
	"time"
)

// CronItem represents a single cron schedule entry (user-facing configuration)
type CronItem struct {
	// Task identifier to execute
	Task string `json:"task"`

	// Cron pattern (e.g., "0 4 * * *" for daily at 4 AM UTC)
	Pattern string `json:"pattern"`

	// Optional unique identifier (defaults to task name)
	Identifier string `json:"identifier,omitempty"`

	// Scheduling options
	Options CronItemOptions `json:"options,omitempty"`

	// Payload to merge with default cron payload
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// DBCronItem represents a cron item as stored in the database
type DBCronItem struct {
	Task        string   `json:"task"`
	Payload     string   `json:"payload"` // JSON string from database
	QueueName   *string  `json:"queue_name"`
	JobKey      *string  `json:"job_key"`
	JobKeyMode  *string  `json:"job_key_mode"`
	Priority    *int     `json:"priority"`
	MaxAttempts *int     `json:"max_attempts"`
	Flags       []string `json:"flags"`
	Fill        string   `json:"fill"`    // Cron pattern
	Options     string   `json:"options"` // JSON string
}

// CronItemOptions configures cron item behavior
type CronItemOptions struct {
	// How far back (in duration) to backfill jobs when worker starts
	// Only backfills since when the identifier was first used
	BackfillPeriod time.Duration `json:"backfillPeriod,omitempty"`

	// Override default job max_attempts
	MaxAttempts *int `json:"maxAttempts,omitempty"`

	// Set job queue_name for serial execution
	QueueName string `json:"queueName,omitempty"`

	// Set job priority
	Priority *int `json:"priority,omitempty"`

	// Set job key for deduplication
	JobKey string `json:"jobKey,omitempty"`

	// Enable/disable backfill when worker starts
	Backfill bool `json:"backfill,omitempty"`
}

// ParsedCronItem represents a parsed and validated cron item (internal representation)
// This type has strict requirements and should only be constructed via parsing functions
type ParsedCronItem struct {
	// Time components (all arrays must be sorted and contain unique values)
	Minutes []int `json:"minutes"` // 0-59
	Hours   []int `json:"hours"`   // 0-23
	Dates   []int `json:"dates"`   // 1-31
	Months  []int `json:"months"`  // 1-12
	DOWs    []int `json:"dows"`    // 0-6 (Sunday=0)

	// Task configuration
	Task       string                 `json:"task"`
	Identifier string                 `json:"identifier"`
	Options    CronItemOptions        `json:"options"`
	Payload    map[string]interface{} `json:"payload"`

	// Job configuration generated from the cron item
	Job CronJob `json:"job"`
}

// TimestampDigest contains the time components needed for cron matching
type TimestampDigest struct {
	Minute int // 0-59
	Hour   int // 0-23
	Date   int // 1-31
	Month  int // 1-12
	DOW    int // 0-6 (Sunday=0)
}

// String converts the digest to a string representation for storage
func (td TimestampDigest) String() string {
	return fmt.Sprintf("%02d:%02d:%02d:%02d:%02d", td.Minute, td.Hour, td.Date, td.Month, td.DOW)
}

// CronJob represents a job created from a cron schedule
type CronJob struct {
	Task        string                 `json:"task"`
	Payload     map[string]interface{} `json:"payload"`             // Must include _cron metadata
	QueueName   *string                `json:"queueName,omitempty"` // Pointer to match TaskSpec
	RunAt       *time.Time             `json:"runAt,omitempty"`     // Time for internal use
	MaxAttempts *int                   `json:"maxAttempts,omitempty"`
	Priority    *int                   `json:"priority,omitempty"`
	JobKey      *string                `json:"jobKey,omitempty"`     // Pointer to match TaskSpec
	JobKeyMode  *string                `json:"jobKeyMode,omitempty"` // Pointer to match TaskSpec
	Flags       []string               `json:"flags,omitempty"`      // Added to match TaskSpec
}

// ToGraphileWorkerFormat converts CronJob to the format expected by graphile-worker
func (cj CronJob) ToGraphileWorkerFormat() GraphileWorkerCronJob {
	var runAtStr string
	if cj.RunAt != nil {
		runAtStr = cj.RunAt.Format(time.RFC3339)
	}

	return GraphileWorkerCronJob{
		Task:        cj.Task,
		Payload:     cj.Payload,
		QueueName:   cj.QueueName,
		RunAt:       runAtStr,
		MaxAttempts: cj.MaxAttempts,
		Priority:    cj.Priority,
	}
}

// GraphileWorkerCronJob represents the exact format used by graphile-worker (string runAt)
type GraphileWorkerCronJob struct {
	Task        string                 `json:"task"`
	Payload     map[string]interface{} `json:"payload"`
	QueueName   *string                `json:"queueName,omitempty"`
	RunAt       string                 `json:"runAt"` // ISO string format (matches graphile-worker)
	MaxAttempts *int                   `json:"maxAttempts,omitempty"`
	Priority    *int                   `json:"priority,omitempty"`
}

// CronPayload represents the _cron metadata structure within CronJob.Payload
type CronPayload struct {
	Ts         string `json:"ts"`                   // Timestamp in ISO format
	Backfilled *bool  `json:"backfilled,omitempty"` // Whether this job was backfilled
}

// JobAndCronIdentifier pairs a cron job with its identifier for database operations
type JobAndCronIdentifier struct {
	Job        CronJob `json:"job"`
	Identifier string  `json:"identifier"`
}

// KnownCrontab represents a record from the known_crontabs table
type KnownCrontab struct {
	Identifier    string     `json:"identifier"`
	KnownSince    time.Time  `json:"known_since"`
	LastExecution *time.Time `json:"last_execution,omitempty"`
}

// WatchedCronItems provides cron items with a release function for cleanup
type WatchedCronItems struct {
	Items   []ParsedCronItem `json:"items"`
	Release func()           `json:"-"` // Function not serializable
}

// Cron provides a simplified interface for cron operations (matches graphile-worker)
type Cron interface {
	// Release stops the cron scheduler and releases resources
	Release() error

	// Promise returns a channel that closes when the cron scheduler stops
	Promise() <-chan struct{}
}

// Scheduler defines the interface for cron scheduling
type Scheduler interface {
	// Start begins the cron scheduler
	Start(ctx context.Context) error

	// Stop gracefully shuts down the scheduler
	Stop() error

	// ReloadCronItems reloads cron items from the database
	ReloadCronItems() error

	// GetCronItems returns current cron items
	GetCronItems() []ParsedCronItem
}

// Parser defines the interface for cron parsing
type Parser interface {
	// ParseCrontab parses a crontab string into cron items
	ParseCrontab(crontab string) ([]ParsedCronItem, error)

	// ParseCronItems converts CronItem slice to ParsedCronItem slice
	ParseCronItems(items []CronItem) ([]ParsedCronItem, error)

	// ParseCronItem converts a single CronItem to ParsedCronItem
	ParseCronItem(item CronItem) (ParsedCronItem, error)

	// ParseOptions parses JSON options string
	ParseOptions(optionsJSON string) (CronItemOptions, error)

	// ValidatePattern validates a cron pattern
	ValidatePattern(pattern string) error
}

// Matcher defines the interface for cron pattern matching
type Matcher interface {
	// Matches returns true if the cron item should fire for the given timestamp
	Matches(item ParsedCronItem, t time.Time) bool

	// DigestTimestamp extracts time components from a timestamp
	DigestTimestamp(t time.Time) TimestampDigest

	// GetScheduleTimesInRange finds all matching times in a range
	GetScheduleTimesInRange(item ParsedCronItem, start, end time.Time) []time.Time
}
