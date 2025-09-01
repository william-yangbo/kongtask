package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/pkg/logger"
)

// CompiledSharedOptions represents processed shared options with compiled values
// This mirrors graphile-worker's CompiledSharedOptions interface
type CompiledSharedOptions struct {
	Logger              *logger.Logger
	WorkerSchema        string
	EscapedWorkerSchema string
	MaxContiguousErrors int
	MaxPoolSize         int
	PollInterval        time.Duration
	Concurrency         int
}

// ProcessSharedOptionsSettings provides settings for processing shared options
type ProcessSharedOptionsSettings struct {
	Scope *logger.LogScope
}

// sharedOptionsCache provides thread-safe caching for compiled shared options
// This implements functionality similar to graphile-worker's WeakMap cache
var (
	sharedOptionsCache      = make(map[string]*CompiledSharedOptions)
	sharedOptionsCacheMutex = &sync.RWMutex{}
)

// generateCacheKey creates a unique cache key for the given options
func generateCacheKey(options *WorkerPoolOptions) string {
	return fmt.Sprintf("schema:%s|concurrency:%d|poll:%v|pool:%d|errors:%d",
		options.Schema,
		options.Concurrency,
		options.PollInterval,
		options.MaxPoolSize,
		options.MaxContiguousErrors,
	)
}

// ProcessSharedOptions processes and caches shared options
// This mirrors graphile-worker's processSharedOptions function with caching
func ProcessSharedOptions(options *WorkerPoolOptions, settings *ProcessSharedOptionsSettings) *CompiledSharedOptions {
	if options == nil {
		options = &WorkerPoolOptions{}
	}
	if settings == nil {
		settings = &ProcessSharedOptionsSettings{}
	}

	// Apply defaults
	applyDefaultOptions(options)

	// Check cache first
	cacheKey := generateCacheKey(options)

	sharedOptionsCacheMutex.RLock()
	if cached, exists := sharedOptionsCache[cacheKey]; exists {
		sharedOptionsCacheMutex.RUnlock()

		// If scope is requested, return a new logger with scope
		if settings.Scope != nil {
			return &CompiledSharedOptions{
				Logger:              cached.Logger.Scope(*settings.Scope),
				WorkerSchema:        cached.WorkerSchema,
				EscapedWorkerSchema: cached.EscapedWorkerSchema,
				MaxContiguousErrors: cached.MaxContiguousErrors,
				MaxPoolSize:         cached.MaxPoolSize,
				PollInterval:        cached.PollInterval,
				Concurrency:         cached.Concurrency,
			}
		}
		return cached
	}
	sharedOptionsCacheMutex.RUnlock()

	// Compile new options
	compiled := &CompiledSharedOptions{
		Logger:              options.Logger,
		WorkerSchema:        options.Schema,
		EscapedWorkerSchema: fmt.Sprintf("\"%s\"", options.Schema), // Simple escaping for now
		MaxContiguousErrors: options.MaxContiguousErrors,
		MaxPoolSize:         options.MaxPoolSize,
		PollInterval:        options.PollInterval,
		Concurrency:         options.Concurrency,
	}

	// Cache the compiled options
	sharedOptionsCacheMutex.Lock()
	sharedOptionsCache[cacheKey] = compiled
	sharedOptionsCacheMutex.Unlock()

	// Apply scope if requested
	if settings.Scope != nil {
		return &CompiledSharedOptions{
			Logger:              compiled.Logger.Scope(*settings.Scope),
			WorkerSchema:        compiled.WorkerSchema,
			EscapedWorkerSchema: compiled.EscapedWorkerSchema,
			MaxContiguousErrors: compiled.MaxContiguousErrors,
			MaxPoolSize:         compiled.MaxPoolSize,
			PollInterval:        compiled.PollInterval,
			Concurrency:         compiled.Concurrency,
		}
	}

	return compiled
}

// Releasers manages cleanup functions
type Releasers []func() error

// Add adds a cleanup function to the releasers
func (r *Releasers) Add(fn func() error) {
	*r = append(*r, fn)
}

// Release executes all cleanup functions
func (r *Releasers) Release() error {
	var errors []string

	// Execute in reverse order (LIFO)
	for i := len(*r) - 1; i >= 0; i-- {
		if err := (*r)[i](); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("release errors: %v", errors)
	}
	return nil
}

// WithReleasersContext executes callback with automatic resource cleanup
// This mirrors graphile-worker's withReleasers function
func WithReleasersContext(ctx context.Context, callback func(context.Context, *Releasers) error) error {
	releasers := &Releasers{}

	defer func() {
		if err := releasers.Release(); err != nil {
			// Log but don't override the main error
			fmt.Printf("Warning: Failed to release resources: %v\n", err)
		}
	}()

	return callback(ctx, releasers)
}

// AssertPool validates and creates a database pool with performance warnings
// This mirrors graphile-worker's assertPool function
func AssertPool(options *WorkerPoolOptions, releasers *Releasers) (*pgxpool.Pool, error) {
	compiled := ProcessSharedOptions(options, nil)

	// Validate connection configuration
	if options.DatabaseURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	// Create pool configuration
	poolConfig, err := pgxpool.ParseConfig(options.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Set max connections
	poolConfig.MaxConns = int32(compiled.MaxPoolSize)
	poolConfig.MinConns = 1

	// Create the pool
	pool, err := pgxpool.New(context.Background(), poolConfig.ConnString())
	if err != nil {
		return nil, fmt.Errorf("failed to create database pool: %w", err)
	}

	// Add cleanup function
	releasers.Add(func() error {
		pool.Close()
		return nil
	})

	// Performance warning - this mirrors graphile-worker's warning
	if compiled.MaxPoolSize < compiled.Concurrency {
		compiled.Logger.Warn(
			fmt.Sprintf("WARNING: having maxPoolSize (%d) smaller than concurrency (%d) may lead to non-optimal performance.",
				compiled.MaxPoolSize, compiled.Concurrency),
		)
	}

	// Add error handler for pool
	// Note: pgx v5 doesn't have pool.on('error') like node-postgres
	// We could implement monitoring in the future if needed

	return pool, nil
}

// UtilsAndReleasers contains compiled utilities and cleanup functions
type UtilsAndReleasers struct {
	*CompiledSharedOptions
	PgPool  *pgxpool.Pool
	Release func() error
	AddJob  AddJobFunc // Will be implemented when we add job utilities
}

// AddJobFunc represents a function for adding jobs to the queue
type AddJobFunc func(ctx context.Context, taskName string, payload interface{}, options *AddJobOptions) error

// AddJobOptions represents options for adding jobs
type AddJobOptions struct {
	QueueName   *string
	RunAt       *time.Time
	MaxAttempts *int
	JobKey      *string
	Priority    *int
}

// GetUtilsAndReleasersFromOptions creates compiled utilities from options
// This mirrors graphile-worker's getUtilsAndReleasersFromOptions function
func GetUtilsAndReleasersFromOptions(options *WorkerPoolOptions, settings *ProcessSharedOptionsSettings) (*UtilsAndReleasers, error) {
	compiled := ProcessSharedOptions(options, settings)

	var result *UtilsAndReleasers

	err := WithReleasersContext(context.Background(), func(ctx context.Context, releasers *Releasers) error {
		// Create and validate database pool
		pgPool, err := AssertPool(options, releasers)
		if err != nil {
			return err
		}

		// Run database migrations
		migrator := migrate.NewMigrator(pgPool, compiled.WorkerSchema)
		if err := migrator.Migrate(ctx); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}

		// Create add job function (placeholder for now)
		addJob := func(ctx context.Context, taskName string, payload interface{}, options *AddJobOptions) error {
			// TODO: Implement actual job addition logic
			return fmt.Errorf("addJob function not yet implemented")
		}

		result = &UtilsAndReleasers{
			CompiledSharedOptions: compiled,
			PgPool:                pgPool,
			Release:               releasers.Release,
			AddJob:                addJob,
		}

		return nil
	})

	return result, err
}

// ClearCache clears the shared options cache (useful for testing)
func ClearCache() {
	sharedOptionsCacheMutex.Lock()
	defer sharedOptionsCacheMutex.Unlock()

	sharedOptionsCache = make(map[string]*CompiledSharedOptions)
}
