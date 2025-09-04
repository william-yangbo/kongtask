package cron

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/events"
	"github.com/william-yangbo/kongtask/pkg/logger"
	"github.com/william-yangbo/kongtask/pkg/worker"
) // DefaultScheduler is the main implementation of the Scheduler interface
type DefaultScheduler struct {
	pgPool  *pgxpool.Pool
	schema  string
	parser  Parser
	matcher Matcher
	logger  *logger.Logger
	events  *events.EventBus

	// Internal state
	mu          sync.RWMutex
	running     bool
	ctx         context.Context
	cancel      context.CancelFunc
	items       []ParsedCronItem
	workerUtils *worker.WorkerUtils

	// Configuration
	backfillOnStart    bool
	clockSkewTolerance time.Duration
	timeProvider       TimeProvider // Add time provider for testability
	useNodeTime        bool         // Use Node.js time instead of PostgreSQL time (sync from graphile-worker cron.ts)
}

// SchedulerConfig contains configuration for the cron scheduler
type SchedulerConfig struct {
	PgPool             *pgxpool.Pool
	Schema             string
	WorkerUtils        *worker.WorkerUtils
	Events             *events.EventBus
	Logger             *logger.Logger
	BackfillOnStart    bool
	ClockSkewTolerance time.Duration
	CronItems          []ParsedCronItem // Pre-parsed cron items
	TimeProvider       TimeProvider     // Time provider for testability
	UseNodeTime        bool             // Use Node.js time instead of PostgreSQL time (sync from graphile-worker cron.ts)
}

// NewScheduler creates a new cron scheduler
func NewScheduler(config SchedulerConfig) Scheduler {
	if config.ClockSkewTolerance == 0 {
		config.ClockSkewTolerance = 10 * time.Second
	}

	// Use real time provider if none specified
	if config.TimeProvider == nil {
		config.TimeProvider = &RealTimeProvider{}
	}

	return &DefaultScheduler{
		pgPool:             config.PgPool,
		schema:             config.Schema,
		parser:             NewParser(),
		matcher:            NewMatcher(),
		logger:             config.Logger,
		events:             config.Events,
		workerUtils:        config.WorkerUtils,
		backfillOnStart:    config.BackfillOnStart,
		clockSkewTolerance: config.ClockSkewTolerance,
		items:              config.CronItems, // Store pre-parsed cron items
		timeProvider:       config.TimeProvider,
		useNodeTime:        config.UseNodeTime, // Sync from graphile-worker cron.ts useNodeTime
	}
}

// Start implements the Scheduler interface
func (s *DefaultScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("scheduler is already running")
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.running = true

	// Emit starting event
	s.events.Emit(events.EventType("cron:starting"), map[string]interface{}{
		"timestamp": s.timeProvider.Now(),
	})

	// Load cron items from database
	if err := s.loadCronItems(); err != nil {
		s.running = false
		s.cancel()
		return fmt.Errorf("failed to load cron items: %w", err)
	}

	// Register known crontabs in database
	if err := s.registerKnownCrontabs(); err != nil {
		s.running = false
		s.cancel()
		return fmt.Errorf("failed to register known crontabs: %w", err)
	}

	// Perform backfill if requested
	if s.backfillOnStart {
		if err := s.performBackfill(); err != nil {
			s.logger.Warn("Failed to perform backfill on start", logger.LogMeta{
				"error": err.Error(),
			})
		}
	}

	// Start the main scheduling loop
	go s.schedulingLoop()

	s.logger.Info("Emitting cron:started event")
	s.events.Emit(events.EventType("cron:started"), map[string]interface{}{
		"timestamp": s.timeProvider.Now(),
		"itemCount": len(s.items),
	})

	return nil
}

// Stop implements the Scheduler interface
func (s *DefaultScheduler) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	s.cancel()

	s.events.Emit(events.EventType("cron:stopped"), map[string]interface{}{
		"timestamp": s.timeProvider.Now(),
	})

	return nil
}

// ReloadCronItems implements the Scheduler interface
func (s *DefaultScheduler) ReloadCronItems() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loadCronItems()
}

// GetCronItems implements the Scheduler interface
func (s *DefaultScheduler) GetCronItems() []ParsedCronItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent external modification
	items := make([]ParsedCronItem, len(s.items))
	copy(items, s.items)
	return items
}

// schedulingLoop runs the main scheduling logic
func (s *DefaultScheduler) schedulingLoop() {
	// Register time change callback for test scenarios
	s.timeProvider.OnTimeChange(func() {
		// Process tasks when time changes (important for fake timers)
		now := s.timeProvider.Now()
		s.processScheduledTasks(now)
	})

	// Also use ticker for regular processing
	ticker := time.NewTicker(1 * time.Minute) // Check every minute
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Use timeProvider for consistent time
			now := s.timeProvider.Now()
			s.processScheduledTasks(now)
		}
	}
}

// processScheduledTasks processes tasks that should run at the given time
func (s *DefaultScheduler) processScheduledTasks(now time.Time) {
	// Get a snapshot of current items
	s.mu.RLock()
	items := make([]ParsedCronItem, len(s.items))
	copy(items, s.items)
	s.mu.RUnlock()

	for _, item := range items {
		if s.shouldRunTask(item, now) {
			if err := s.scheduleTask(item, now); err != nil {
				s.logger.Error("Failed to schedule task", logger.LogMeta{
					"taskIdentifier": item.Job.Task,
					"error":          err.Error(),
				})
			}
		}
	}
}

// shouldRunTask determines if a task should be scheduled at the given time
func (s *DefaultScheduler) shouldRunTask(item ParsedCronItem, now time.Time) bool {
	// Check if the cron pattern matches the current time
	return s.matcher.Matches(item, now)
}

// scheduleTask schedules a single task
func (s *DefaultScheduler) scheduleTask(item ParsedCronItem, scheduledAt time.Time) error {
	// Generate a timestamp digest for this execution
	digest := s.matcher.DigestTimestamp(scheduledAt).String()

	// Check if this task has already been scheduled for this time
	alreadyScheduled, err := s.isTaskAlreadyScheduled(item, digest)
	if err != nil {
		return fmt.Errorf("failed to check if task is already scheduled: %w", err)
	}
	if alreadyScheduled {
		return nil // Skip if already scheduled
	}

	// Check for timing issues
	timeDiff := s.timeProvider.Now().Sub(scheduledAt)
	if timeDiff > s.clockSkewTolerance {
		// Log debug message for clock skew (aligned with graphile-worker 71d22e9)
		s.logger.Debug("KongTask Cron fired too late; catching up", logger.LogMeta{
			"taskIdentifier": item.Job.Task,
			"scheduledAt":    scheduledAt,
			"actualTime":     s.timeProvider.Now(),
			"delay":          timeDiff,
		})

		s.events.Emit(events.EventType("cron:overdueTimer"), map[string]interface{}{
			"taskIdentifier": item.Job.Task,
			"scheduledAt":    scheduledAt,
			"actualTime":     s.timeProvider.Now(),
			"delay":          timeDiff,
		})
	} else if timeDiff < -s.clockSkewTolerance {
		// Log debug message for clock skew (aligned with graphile-worker 71d22e9)
		s.logger.Debug("KongTask Cron fired too early (clock skew?); proceeding", logger.LogMeta{
			"taskIdentifier": item.Job.Task,
			"scheduledAt":    scheduledAt,
			"actualTime":     s.timeProvider.Now(),
			"advance":        -timeDiff,
		})

		s.events.Emit(events.EventType("cron:prematureTimer"), map[string]interface{}{
			"taskIdentifier": item.Job.Task,
			"scheduledAt":    scheduledAt,
			"actualTime":     s.timeProvider.Now(),
			"advance":        -timeDiff,
		})
	}

	// Record that we're scheduling this task
	if err := s.recordScheduledTask(item, digest, scheduledAt); err != nil {
		return fmt.Errorf("failed to record scheduled task: %w", err)
	}

	// Emit scheduling event
	s.events.Emit(events.EventType("cron:schedule"), map[string]interface{}{
		"taskIdentifier": item.Job.Task,
		"scheduledAt":    scheduledAt,
		"digest":         digest,
	})

	// Schedule the actual job
	if err := s.addJobToQueue(item, scheduledAt); err != nil {
		return fmt.Errorf("failed to add job to queue: %w", err)
	}

	s.events.Emit(events.EventType("cron:scheduled"), map[string]interface{}{
		"taskIdentifier": item.Job.Task,
		"scheduledAt":    scheduledAt,
		"digest":         digest,
	})

	return nil
} // loadCronItems loads cron items from the database
func (s *DefaultScheduler) loadCronItems() error {
	// If we have pre-parsed cron items, use them directly
	if len(s.items) > 0 {
		s.logger.Debug("Using pre-parsed cron items", logger.LogMeta{
			"count": len(s.items),
		})
		return nil
	}

	// If no pre-parsed items, this scheduler instance has no cron jobs to manage
	s.logger.Debug("No cron items to load", logger.LogMeta{})
	return nil
}

// registerKnownCrontabs registers the cron identifiers in the database
// This matches the TypeScript implementation in graphile-worker
func (s *DefaultScheduler) registerKnownCrontabs() error {
	if len(s.items) == 0 {
		return nil
	}

	// Prepare batch insert/update for known crontabs
	for _, item := range s.items {
		query := fmt.Sprintf(`
			INSERT INTO %s.known_crontabs (identifier, known_since)
			VALUES ($1, NOW())
			ON CONFLICT (identifier) DO NOTHING
		`, s.schema)

		_, err := s.pgPool.Exec(context.Background(), query, item.Identifier)
		if err != nil {
			return fmt.Errorf("failed to register crontab %s: %w", item.Identifier, err)
		}

		s.logger.Debug("Registered crontab identifier", logger.LogMeta{
			"identifier": item.Identifier,
		})
	}

	return nil
}

// getLastExecutionTime retrieves the last execution time for a specific identifier
func (s *DefaultScheduler) getLastExecutionTime(identifier string) (*time.Time, error) {
	conn, err := s.pgPool.Acquire(s.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire database connection: %w", err)
	}
	defer conn.Release()

	query := fmt.Sprintf(`
		SELECT last_execution
		FROM %s.known_crontabs
		WHERE identifier = $1
	`, s.schema)

	var lastExecution *time.Time
	err = conn.QueryRow(s.ctx, query, identifier).Scan(&lastExecution)
	if err != nil {
		if err.Error() == "no rows in result set" {
			// No record found, return nil (no previous execution)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query last execution time: %w", err)
	}

	return lastExecution, nil
}

// performBackfill performs backfill for missed cron jobs
func (s *DefaultScheduler) performBackfill() error {
	s.logger.Info("Starting cron backfill")

	now := s.timeProvider.Now()

	for _, item := range s.items {
		if !item.Options.Backfill {
			continue // Skip items that don't want backfill
		}

		s.logger.Debug("Processing backfill for item", logger.LogMeta{
			"identifier": item.Identifier,
		})

		// Get the last execution time from database for this identifier
		lastExecution, err := s.getLastExecutionTime(item.Identifier)
		if err != nil {
			s.logger.Error("Failed to get last execution time", logger.LogMeta{
				"identifier": item.Identifier,
				"error":      err.Error(),
			})
			continue
		}

		var backfillStart time.Time
		if lastExecution != nil {
			// Start backfill from the last execution time
			backfillStart = *lastExecution
			s.logger.Debug("Backfill from last execution", logger.LogMeta{
				"identifier":    item.Identifier,
				"lastExecution": lastExecution.Format(time.RFC3339),
			})
		} else {
			// No previous execution - this is a new cron, skip backfill
			s.logger.Debug("Skipping backfill for new cron identifier (no previous execution)", logger.LogMeta{
				"identifier": item.Identifier,
			})
			continue
		}

		// Ensure we don't backfill beyond the specified period
		maxBackfillStart := now.Add(-item.Options.BackfillPeriod)
		if backfillStart.Before(maxBackfillStart) {
			backfillStart = maxBackfillStart
		}

		if err := s.backfillItem(item, backfillStart, now); err != nil {
			s.logger.Error("Failed to backfill item", logger.LogMeta{
				"task":  item.Job.Task,
				"error": err.Error(),
			})
		}
	}

	s.logger.Info("Cron backfill completed")
	return nil
} // backfillItem performs backfill for a single cron item
func (s *DefaultScheduler) backfillItem(item ParsedCronItem, start, end time.Time) error {
	s.events.Emit(events.EventType("cron:backfill"), map[string]interface{}{
		"taskIdentifier": item.Job.Task,
		"start":          start,
		"end":            end,
	})

	// Find all times this job should have run in the backfill window
	scheduleTimes := s.matcher.GetScheduleTimesInRange(item, start, end)

	for _, scheduleTime := range scheduleTimes {
		// Use the timestamp itself as the "digest" for simplicity
		timeStr := scheduleTime.Format(time.RFC3339)

		// Check if this execution was already recorded
		alreadyScheduled, err := s.isTaskAlreadyScheduled(item, timeStr)
		if err != nil {
			return fmt.Errorf("failed to check if task is already scheduled: %w", err)
		}
		if alreadyScheduled {
			continue // Skip if already scheduled
		}

		// Record and schedule the task
		if err := s.recordScheduledTask(item, timeStr, scheduleTime); err != nil {
			return fmt.Errorf("failed to record scheduled task: %w", err)
		}

		if err := s.addJobToQueue(item, scheduleTime); err != nil {
			return fmt.Errorf("failed to add backfill job to queue: %w", err)
		}
	}

	return nil
}

// isTaskAlreadyScheduled checks if a task has already been scheduled for a given digest
func (s *DefaultScheduler) isTaskAlreadyScheduled(item ParsedCronItem, digest string) (bool, error) {
	conn, err := s.pgPool.Acquire(s.ctx)
	if err != nil {
		return false, fmt.Errorf("failed to acquire database connection: %w", err)
	}
	defer conn.Release()

	// Parse the digest to get the scheduled time components
	digestTime := s.matcher.DigestTimestamp(s.timeProvider.Now())

	// Check if there's already a record for this cron item that matches the current digest time
	query := fmt.Sprintf(`
		SELECT EXISTS(
			SELECT 1 FROM %s.known_crontabs 
			WHERE identifier = $1 
			AND last_execution IS NOT NULL
			AND EXTRACT(MINUTE FROM last_execution) = $2
			AND EXTRACT(HOUR FROM last_execution) = $3
			AND EXTRACT(DAY FROM last_execution) = $4
			AND EXTRACT(MONTH FROM last_execution) = $5
			AND EXTRACT(DOW FROM last_execution) = $6
		)
	`, s.schema)

	var exists bool
	err = conn.QueryRow(s.ctx, query, item.Identifier,
		digestTime.Minute, digestTime.Hour, digestTime.Date, digestTime.Month, digestTime.DOW).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check task existence: %w", err)
	}

	return exists, nil
}

// recordScheduledTask records that a task has been scheduled for a given digest
func (s *DefaultScheduler) recordScheduledTask(item ParsedCronItem, digest string, scheduledAt time.Time) error {
	conn, err := s.pgPool.Acquire(s.ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire database connection: %w", err)
	}
	defer conn.Release()

	// Update the last_execution timestamp to track the latest scheduled time
	query := fmt.Sprintf(`
		UPDATE %s.known_crontabs 
		SET last_execution = $2
		WHERE identifier = $1
		AND (last_execution IS NULL OR last_execution < $2)
	`, s.schema)

	_, err = conn.Exec(s.ctx, query, item.Identifier, scheduledAt)
	if err != nil {
		return fmt.Errorf("failed to record scheduled task: %w", err)
	}

	return nil
}

// addJobToQueue adds a job to the worker queue
func (s *DefaultScheduler) addJobToQueue(item ParsedCronItem, scheduledAt time.Time) error {
	// Create TaskSpec for the job, handling pointer fields correctly
	// Note: scheduledAt is already calculated using Go time source when useNodeTime is true,
	// which synchronizes with graphile-worker cron.ts useNodeTime behavior
	spec := worker.TaskSpec{
		RunAt:       &scheduledAt,         // Use the calculated scheduled time (respects useNodeTime in calculation)
		QueueName:   item.Job.QueueName,   // These are already *string in CronItem
		Priority:    item.Job.Priority,    // Already *int
		MaxAttempts: item.Job.MaxAttempts, // Already *int
		JobKey:      item.Job.JobKey,      // Already *string
		JobKeyMode:  item.Job.JobKeyMode,  // Already *string
		Flags:       item.Job.Flags,       // Already []string
	}

	// Use QuickAddJob method which doesn't require a transaction
	_, err := s.workerUtils.QuickAddJob(s.ctx, item.Job.Task, item.Job.Payload, spec)
	if err != nil {
		return fmt.Errorf("failed to add job to queue: %w", err)
	}

	return nil
}
