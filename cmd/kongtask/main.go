package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/pkg/cron"
	"github.com/william-yangbo/kongtask/pkg/logger"
	"github.com/william-yangbo/kongtask/pkg/worker"
)

// safeInt32 safely converts an int to int32, returning an error if overflow would occur
func safeInt32(val int) (int32, error) {
	if val > math.MaxInt32 {
		return 0, fmt.Errorf("value %d exceeds maximum allowed value %d", val, math.MaxInt32)
	}
	return int32(val), nil //#nosec G115 -- Safe conversion, we check for overflow above
}

var (
	cfgFile              string
	databaseURL          string
	schema               string
	schemaOnly           bool
	once                 bool
	watch                bool // [EXPERIMENTAL] Watch task files for changes (sync from cli.ts)
	jobs                 int
	maxPoolSize          int
	pollInterval         int
	noHandleSignals      bool
	noPreparedStatements bool
	taskDirectory        string // Directory containing task files (sync from cli.ts)
	crontabFile          string // Path to crontab file (sync from cli.ts)
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "kongtask",
	Short: "A Go implementation of graphile-worker v0.2.0",
	Long: `kongtask is a Go implementation of the original graphile-worker v0.2.0,
providing background job processing for PostgreSQL databases.

This implementation maintains strict compatibility with the original TypeScript
version's schema and behavior.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Validate mutually exclusive flags (sync from cli.ts validation)
		if schemaOnly && watch {
			log.Fatal("Cannot specify both --watch and --schema-only")
		}
		if schemaOnly && once {
			log.Fatal("Cannot specify both --once and --schema-only")
		}
		if watch && once {
			log.Fatal("Cannot specify both --watch and --once")
		}

		if schemaOnly {
			if err := runMigrate(); err != nil {
				log.Fatalf("Schema migration failed: %v", err)
			}
			fmt.Println("Schema migration completed successfully")
			return
		}

		if once {
			if err := runWorkerOnce(); err != nil {
				log.Fatalf("Worker failed: %v", err)
			}
			return
		}

		if err := runWorker(); err != nil {
			log.Fatalf("Worker failed: %v", err)
		}
	},
}

// migrateCmd represents the migrate command
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations",
	Long: `Run database migrations to set up the graphile_worker schema.

This command creates all necessary tables, functions, and triggers required
for job processing.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runMigrate(); err != nil {
			log.Fatalf("Migration failed: %v", err)
		}
		fmt.Println("Migration completed successfully")
	},
}

// workerCmd represents the worker command
var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Start the job worker",
	Long: `Start the job worker to process jobs from the queue.

The worker will continuously poll for jobs and process them using
registered task handlers.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runWorker(); err != nil {
			log.Fatalf("Worker failed: %v", err)
		}
	},
}

// runTaskListCmd represents the run-task-list command (matches TypeScript runTaskList)
var runTaskListCmd = &cobra.Command{
	Use:   "run-task-list",
	Short: "Start a worker pool to continuously process jobs (equivalent to graphile-worker runTaskList)",
	Long: `Start a worker pool to continuously process jobs. This is equivalent to graphile-worker's 
runTaskList function. It starts multiple workers and runs continuously until terminated,
making it suitable for long-running job processing services.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runTaskListPool(); err != nil {
			log.Fatalf("RunTaskList failed: %v", err)
		}
	},
}

// runTaskListOnceCmd represents the run-task-list-once command
var runTaskListOnceCmd = &cobra.Command{
	Use:   "run-task-list-once",
	Short: "Run all available jobs once and exit (equivalent to graphile-worker runTaskListOnce)",
	Long: `Run all available jobs once and exit. This is equivalent to graphile-worker's 
runTaskListOnce function. It processes all currently available jobs and then exits,
making it suitable for one-time job processing or scheduled runs.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runTaskListOnce(); err != nil {
			log.Fatalf("RunTaskListOnce failed: %v", err)
		}
	},
}

// generateConfigCmd generates a sample configuration file
var generateConfigCmd = &cobra.Command{
	Use:   "generate-config [filename]",
	Short: "Generate a sample configuration file",
	Long: `Generate a sample configuration file with all available options and their default values.
This helps users understand the available configuration options and their format.

By default, generates 'kongtask.json' in the current directory.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filename := "kongtask.json"
		if len(args) > 0 {
			filename = args[0]
		}

		if err := worker.SaveSampleConfiguration(filename); err != nil {
			log.Fatalf("Failed to generate config file: %v", err)
		}

		fmt.Printf("Sample configuration file generated: %s\n", filename)
		fmt.Println("\nYou can now:")
		fmt.Println("1. Edit the configuration file with your settings")
		fmt.Println("2. Use --config flag to specify the config file")
		fmt.Println("3. Or place it in your home directory as ~/.kongtask.json")
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.kongtask.yaml)")
	rootCmd.PersistentFlags().StringVar(&databaseURL, "database-url", "", "PostgreSQL connection URL")
	rootCmd.PersistentFlags().StringVar(&schema, "schema", "graphile_worker", "Schema name for worker tables")

	// v0.2.0 flags
	rootCmd.Flags().BoolVar(&schemaOnly, "schema-only", false, "Just install (or update) the database schema, then exit")
	rootCmd.Flags().BoolVar(&once, "once", false, "Run until there are no runnable jobs left, then exit")
	rootCmd.Flags().BoolVarP(&watch, "watch", "w", false, "[EXPERIMENTAL] Watch task files for changes, automatically reloading the task code without restarting worker")

	// Task and cron configuration (sync from cli.ts)
	rootCmd.Flags().StringVar(&taskDirectory, "task-directory", "./tasks", "Directory containing task files")
	rootCmd.Flags().StringVar(&crontabFile, "crontab-file", "./crontab", "Path to crontab file")

	// Worker configuration flags (matching graphile-worker CLI)
	rootCmd.Flags().IntVarP(&jobs, "jobs", "j", worker.ConcurrentJobs, "number of jobs to run concurrently")
	rootCmd.Flags().IntVarP(&maxPoolSize, "max-pool-size", "m", worker.DefaultMaxPoolSize, "maximum size of the PostgreSQL pool")
	rootCmd.Flags().IntVar(&pollInterval, "poll-interval", int(worker.DefaultPollInterval.Milliseconds()), "how long to wait between polling for jobs in milliseconds (for jobs scheduled in the future/retries)")
	rootCmd.Flags().BoolVar(&noHandleSignals, "no-handle-signals", false, "if set, we won't install signal handlers and it'll be up to you to handle graceful shutdown")
	rootCmd.Flags().BoolVar(&noPreparedStatements, "no-prepared-statements", false, "set this flag if you want to disable prepared statements, e.g. for compatibility with pgBouncer")

	// Add connection alias for compatibility with graphile-worker
	rootCmd.PersistentFlags().StringVarP(&databaseURL, "connection", "c", "", "Database connection string (alias for --database-url)")

	// Bind flags to viper
	_ = viper.BindPFlag("database_url", rootCmd.PersistentFlags().Lookup("database-url"))
	_ = viper.BindPFlag("connection", rootCmd.PersistentFlags().Lookup("connection"))
	_ = viper.BindPFlag("schema", rootCmd.PersistentFlags().Lookup("schema"))
	_ = viper.BindPFlag("jobs", rootCmd.Flags().Lookup("jobs"))
	_ = viper.BindPFlag("max_pool_size", rootCmd.Flags().Lookup("max-pool-size"))
	_ = viper.BindPFlag("poll_interval", rootCmd.Flags().Lookup("poll-interval"))
	_ = viper.BindPFlag("no_prepared_statements", rootCmd.Flags().Lookup("no-prepared-statements"))

	// Add subcommands
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(workerCmd)
	rootCmd.AddCommand(runTaskListCmd)
	rootCmd.AddCommand(runTaskListOnceCmd)
	rootCmd.AddCommand(generateConfigCmd)
}

// initConfig reads in config file and ENV variables if set.
// Enhanced to support the new configuration system with priority handling
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in multiple locations following graphile-worker pattern
		viper.AddConfigPath(".")               // current directory
		viper.AddConfigPath(home)              // home directory
		viper.AddConfigPath(home + "/.config") // ~/.config directory

		// Support multiple config formats
		viper.SetConfigName("kongtask")  // name of config file (without extension)
		viper.SetConfigName(".kongtask") // hidden config file
	}

	// Enable environment variable support with GRAPHILE_WORKER_ prefix
	viper.SetEnvPrefix("GRAPHILE_WORKER")
	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

func runMigrate() error {
	ctx := context.Background()

	dbURL := viper.GetString("database_url")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" && os.Getenv("PGDATABASE") == "" {
		return fmt.Errorf("database URL is required (use --database-url flag, DATABASE_URL env var, or PG* env vars including at least PGDATABASE)")
	}

	schemaName := viper.GetString("schema")
	if schemaName == "" {
		schemaName = "graphile_worker"
	}

	// Create connection pool
	var pool *pgxpool.Pool
	var err error

	if dbURL != "" {
		pool, err = pgxpool.New(ctx, dbURL)
	} else {
		// Use PG* environment variables
		pool, err = pgxpool.New(ctx, "")
	}
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}
	defer pool.Close()

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Run migration
	migrator := migrate.NewMigrator(pool, schemaName)
	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	return nil
}

func runWorker() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Received shutdown signal, stopping worker...")
		cancel()
	}()

	dbURL := viper.GetString("database_url")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" && os.Getenv("PGDATABASE") == "" {
		return fmt.Errorf("database URL is required (use --database-url flag, DATABASE_URL env var, or PG* env vars including at least PGDATABASE)")
	}

	schemaName := viper.GetString("schema")
	if schemaName == "" {
		schemaName = "graphile_worker"
	}

	// Load tasks and cron items (sync from cli.ts main function)
	watchedTasks, err := getTasks(taskDirectory, watch)
	if err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}
	defer watchedTasks.Release()

	watchedCronItems, err := getCronItems(crontabFile, watch)
	if err != nil {
		return fmt.Errorf("failed to load cron items: %w", err)
	}
	defer watchedCronItems.Release()

	// If we have both tasks and cron items, use the runner package for full integration
	if len(watchedTasks.Tasks) > 0 || len(watchedCronItems.Items) > 0 {
		return runWithRunner(ctx, watchedTasks.Tasks, watchedCronItems.Items, schemaName)
	}

	// Fallback to simple worker mode if no tasks or cron items
	return runSimpleWorker(ctx)
}

// runWithRunner uses the runner package for full task and cron integration (sync from cli.ts)
func runWithRunner(ctx context.Context, tasks map[string]worker.TaskHandler, cronItems []cron.ParsedCronItem, schemaName string) error {
	// TODO: Implement runner integration with tasks and cron items
	// This should use the runner package we created earlier
	_ = schemaName // Placeholder until implementation is complete
	return fmt.Errorf("runner integration not yet implemented")
}

// runSimpleWorker runs a basic worker without task loading (original implementation)
func runSimpleWorker(ctx context.Context) error {
	dbURL := viper.GetString("database_url")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" && os.Getenv("PGDATABASE") == "" {
		return fmt.Errorf("database URL is required")
	}

	schemaName := viper.GetString("schema")
	if schemaName == "" {
		schemaName = "graphile_worker"
	}

	// Get CLI parameters or use defaults (concurrency not needed for single worker)
	maxPool := viper.GetInt("max_pool_size")
	if maxPool <= 0 {
		maxPool = maxPoolSize
		if maxPool <= 0 {
			maxPool = worker.DefaultMaxPoolSize
		}
	}

	pollIntervalMs := viper.GetInt("poll_interval")
	if pollIntervalMs <= 0 {
		pollIntervalMs = pollInterval
		if pollIntervalMs <= 0 {
			pollIntervalMs = int(worker.DefaultPollInterval.Milliseconds())
		}
	}
	pollDuration := time.Duration(pollIntervalMs) * time.Millisecond

	// Create connection pool with custom max size
	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return fmt.Errorf("failed to parse database URL: %w", err)
	}
	maxConns, err := safeInt32(maxPool)
	if err != nil {
		return fmt.Errorf("invalid max pool size: %w", err)
	}
	poolConfig.MaxConns = maxConns

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}
	defer pool.Close()

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Create worker with custom configuration
	customLogger := logger.NewLogger(logger.ConsoleLogFactory)
	w := worker.NewWorker(pool, schemaName,
		worker.WithLogger(customLogger),
		worker.WithPollInterval(pollDuration))

	// Register example task handler (v0.2.0 signature)
	w.RegisterTask("example_task", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		helpers.Logger.Info(fmt.Sprintf("Processing example task with payload: %s", string(payload)))
		return nil
	})

	log.Printf("Starting worker with schema: %s, poll interval: %v, max pool size: %d",
		schemaName, pollDuration, maxPool)

	// Run worker
	if err := w.Run(ctx); err != nil && err != context.Canceled {
		return fmt.Errorf("worker failed: %w", err)
	}

	log.Println("Worker stopped gracefully")
	return nil
}

func runWorkerOnce() error {
	ctx := context.Background()

	dbURL := viper.GetString("database_url")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" && os.Getenv("PGDATABASE") == "" {
		return fmt.Errorf("database URL is required (use --database-url flag, DATABASE_URL env var, or PG* env vars including at least PGDATABASE)")
	}

	schemaName := viper.GetString("schema")
	if schemaName == "" {
		schemaName = "graphile_worker"
	}

	// Get CLI parameters or use defaults
	maxPool := viper.GetInt("max_pool_size")
	if maxPool <= 0 {
		maxPool = maxPoolSize
		if maxPool <= 0 {
			maxPool = worker.DefaultMaxPoolSize
		}
	}

	pollIntervalMs := viper.GetInt("poll_interval")
	if pollIntervalMs <= 0 {
		pollIntervalMs = pollInterval
		if pollIntervalMs <= 0 {
			pollIntervalMs = int(worker.DefaultPollInterval.Milliseconds())
		}
	}
	pollDuration := time.Duration(pollIntervalMs) * time.Millisecond

	// Create connection pool with custom max size
	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return fmt.Errorf("failed to parse database URL: %w", err)
	}
	maxConns, err := safeInt32(maxPool)
	if err != nil {
		return fmt.Errorf("invalid max pool size: %w", err)
	}
	poolConfig.MaxConns = maxConns

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}
	defer pool.Close()

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Create worker with custom configuration
	customLogger := logger.NewLogger(logger.ConsoleLogFactory)
	w := worker.NewWorker(pool, schemaName,
		worker.WithLogger(customLogger),
		worker.WithPollInterval(pollDuration))

	// Register example task handler (v0.2.0 signature)
	w.RegisterTask("example_task", func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
		helpers.Logger.Info(fmt.Sprintf("Processing example task with payload: %s", string(payload)))
		return nil
	})

	log.Printf("Starting worker (once mode) with schema: %s, poll interval: %v, max pool size: %d",
		schemaName, pollDuration, maxPool)

	// Run worker once - process all available jobs then exit
	return w.RunOnce(ctx)
}

// runTaskListOnce runs all available jobs once using the new RunTaskListOnce API
func runTaskListOnce() error {
	ctx := context.Background()

	dbURL := viper.GetString("database_url")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" && os.Getenv("PGDATABASE") == "" {
		return fmt.Errorf("database URL is required (use --database-url flag, DATABASE_URL env var, or PG* env vars including at least PGDATABASE)")
	}

	schemaName := viper.GetString("schema")
	if schemaName == "" {
		schemaName = "graphile_worker"
	}

	// Get CLI parameters or use defaults
	maxPool := viper.GetInt("max_pool_size")
	if maxPool <= 0 {
		maxPool = maxPoolSize
		if maxPool <= 0 {
			maxPool = worker.DefaultMaxPoolSize
		}
	}

	// Create connection pool with custom max size
	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return fmt.Errorf("failed to parse database URL: %w", err)
	}
	maxConns, err := safeInt32(maxPool)
	if err != nil {
		return fmt.Errorf("invalid max pool size: %w", err)
	}
	poolConfig.MaxConns = maxConns

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}
	defer pool.Close()

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Define task handlers (equivalent to graphile-worker TaskList)
	tasks := map[string]worker.TaskHandler{
		"example_task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			helpers.Logger.Info(fmt.Sprintf("Processing example task with payload: %s", string(payload)))
			return nil
		},
		"test_task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			helpers.Logger.Info(fmt.Sprintf("Processing test task with payload: %s", string(payload)))
			return nil
		},
	}

	// Create options for RunTaskListOnce
	options := worker.RunTaskListOnceOptions{
		Schema: schemaName,
		Logger: logger.NewLogger(logger.ConsoleLogFactory),
	}

	log.Printf("Starting RunTaskListOnce with schema: %s, max pool size: %d", schemaName, maxPool)

	// Run all available jobs once and exit
	return worker.RunTaskListOnce(ctx, tasks, pool, options)
}

// runTaskListPool runs a worker pool continuously (equivalent to graphile-worker runTaskList)
func runTaskListPool() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration with priority: CLI flags > Environment > Config file > Defaults
	cliOverrides := map[string]interface{}{}

	// Collect CLI overrides (only if explicitly set)
	if databaseURL != "" {
		cliOverrides["database_url"] = databaseURL
	}
	if schema != "" {
		cliOverrides["schema"] = schema
	}
	if jobs > 0 {
		cliOverrides["concurrency"] = jobs
	}
	if maxPoolSize > 0 {
		cliOverrides["max_pool_size"] = maxPoolSize
	}
	if pollInterval > 0 {
		cliOverrides["poll_interval"] = time.Duration(pollInterval) * time.Millisecond
	}
	if noPreparedStatements {
		cliOverrides["no_prepared_statements"] = noPreparedStatements
	}

	// Load configuration with priority system
	config, err := worker.LoadConfigurationWithOverrides(cliOverrides)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate configuration
	if err := config.ValidateConfiguration(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Use database URL from config or fallback to environment
	dbURL := config.DatabaseURL
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" && os.Getenv("PGDATABASE") == "" {
		return fmt.Errorf("database URL is required (use --database-url flag, config file, DATABASE_URL env var, or PG* env vars including at least PGDATABASE)")
	}

	// Create connection pool with custom max size
	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return fmt.Errorf("failed to parse database URL: %w", err)
	}
	maxConns, err := safeInt32(config.MaxPoolSize)
	if err != nil {
		return fmt.Errorf("invalid max pool size: %w", err)
	}
	poolConfig.MaxConns = maxConns

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}
	defer pool.Close()

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Define task handlers (equivalent to graphile-worker TaskList)
	tasks := map[string]worker.TaskHandler{
		"example_task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			helpers.Logger.Info(fmt.Sprintf("Processing example task with payload: %s", string(payload)))
			return nil
		},
		"test_task": func(ctx context.Context, payload json.RawMessage, helpers *worker.Helpers) error {
			helpers.Logger.Info(fmt.Sprintf("Processing test task with payload: %s", string(payload)))
			return nil
		},
	}

	// Create worker pool options from configuration
	options := config.ToWorkerPoolOptions()

	// Override no handle signals flag based on CLI
	if noHandleSignals {
		options.NoHandleSignals = true
	}

	log.Printf("Starting worker pool with schema: %s, concurrency: %d, poll interval: %v, max pool size: %d",
		config.Schema, options.Concurrency, config.PollInterval, config.MaxPoolSize)

	// Start worker pool with signal handling (mirrors TypeScript runTaskList exactly)
	workerPool, err := worker.RunTaskListWithSignalHandling(ctx, options, tasks, pool)
	if err != nil {
		return fmt.Errorf("failed to start worker pool: %w", err)
	}

	// Wait for worker pool to complete (will run until signal received)
	workerPool.Wait()

	log.Println("Worker pool stopped gracefully")
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
