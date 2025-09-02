package runner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/pkg/events"
	"github.com/william-yangbo/kongtask/pkg/logger"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// RunnerOptions holds configuration for creating a runner (v0.4.0 alignment)
type RunnerOptions struct {
	// Task configuration (exactly one must be provided)
	TaskList      map[string]worker.TaskHandler
	TaskDirectory string

	// Database configuration (exactly one must be provided)
	ConnectionString string
	PgPool           *pgxpool.Pool
	MaxPoolSize      int

	// Worker configuration
	Concurrency int
	Logger      *logger.Logger
	Schema      string

	// Advanced options
	PollInterval time.Duration
	WorkerID     string

	// Event system (v0.4.0 alignment)
	Events *events.EventBus
}

// Runner represents a running job processor (v0.4.0 alignment)
type Runner struct {
	worker     *worker.Worker
	pool       *pgxpool.Pool
	ownedPool  bool // Whether we created the pool and should close it
	stopped    bool
	stopMutex  sync.RWMutex
	stopCh     chan struct{}
	completeCh chan error
	releasers  []ReleaseFunc
	events     *events.EventBus // Events emitter for runner events
}

// ReleaseFunc is a function that releases resources
type ReleaseFunc func() error

// Releasers manages automatic resource cleanup (v0.4.0 alignment)
type Releasers struct {
	funcs []ReleaseFunc
	mutex sync.Mutex
}

// Add adds a release function
func (r *Releasers) Add(fn ReleaseFunc) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.funcs = append(r.funcs, fn)
}

// ReleaseAll releases all resources
func (r *Releasers) ReleaseAll() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	var errors []string
	for i := len(r.funcs) - 1; i >= 0; i-- {
		if err := r.funcs[i](); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("release errors: %s", strings.Join(errors, "; "))
	}
	return nil
}

// WithReleasers executes a callback with automatic resource management (v0.4.0 alignment)
func WithReleasers(callback func(*Releasers) error) error {
	releasers := &Releasers{}

	defer func() {
		if err := releasers.ReleaseAll(); err != nil {
			// Log error but don't override callback error
			logger.DefaultLogger.Error(fmt.Sprintf("Failed to release resources: %v", err))
		}
	}()

	return callback(releasers)
}

// validateOptions validates RunnerOptions (v0.4.0 alignment)
func validateOptions(options RunnerOptions) error {
	// Validate task configuration
	if options.TaskList == nil && options.TaskDirectory == "" {
		return fmt.Errorf("exactly one of TaskList or TaskDirectory must be provided")
	}
	if options.TaskList != nil && options.TaskDirectory != "" {
		return fmt.Errorf("both TaskList and TaskDirectory are set, at most one should be provided")
	}

	// Validate database configuration
	if options.PgPool == nil && options.ConnectionString == "" && os.Getenv("DATABASE_URL") == "" {
		return fmt.Errorf("you must either specify PgPool or ConnectionString, or set DATABASE_URL environment variable")
	}
	if options.PgPool != nil && options.ConnectionString != "" {
		return fmt.Errorf("both PgPool and ConnectionString are set, at most one should be provided")
	}

	return nil
}

// setDefaults sets default values for RunnerOptions (v0.4.0 alignment)
func setDefaults(options *RunnerOptions) {
	if options.Concurrency == 0 {
		options.Concurrency = worker.ConcurrentJobs
	}
	if options.MaxPoolSize == 0 {
		options.MaxPoolSize = worker.DefaultMaxPoolSize
	}
	if options.Logger == nil {
		options.Logger = logger.DefaultLogger
	}
	if options.PollInterval == 0 {
		options.PollInterval = 2 * time.Second
	}
	if options.Schema == "" {
		options.Schema = "graphile_worker"
	}
}

// assertPool creates and validates database pool (v0.4.0 alignment)
func assertPool(options RunnerOptions, releasers *Releasers) (*pgxpool.Pool, bool, error) {
	var pool *pgxpool.Pool
	var ownedPool bool

	if options.PgPool != nil {
		pool = options.PgPool
		ownedPool = false
	} else {
		connectionString := options.ConnectionString
		if connectionString == "" {
			connectionString = os.Getenv("DATABASE_URL")
		}

		config, err := pgxpool.ParseConfig(connectionString)
		if err != nil {
			return nil, false, fmt.Errorf("failed to parse connection string: %w", err)
		}

		// Set max pool size
		config.MaxConns = int32(options.MaxPoolSize)

		pool, err = pgxpool.NewWithConfig(context.Background(), config)
		if err != nil {
			return nil, false, fmt.Errorf("failed to create connection pool: %w", err)
		}

		ownedPool = true
		releasers.Add(func() error {
			pool.Close()
			return nil
		})
	}

	// Validate pool size vs concurrency
	maxConns := int(pool.Config().MaxConns)
	if maxConns < options.Concurrency {
		options.Logger.Warn(fmt.Sprintf(
			"WARNING: having maxPoolSize (%d) smaller than concurrency (%d) may lead to non-optimal performance",
			maxConns, options.Concurrency,
		))
	}

	// Add error event handling
	// Note: pgx v5 doesn't have the same error events as node-postgres
	// But we can test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return nil, ownedPool, fmt.Errorf("failed to ping database: %w", err)
	}

	return pool, ownedPool, nil
}
