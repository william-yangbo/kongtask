package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/william-yangbo/kongtask/internal/migrate"
	"github.com/william-yangbo/kongtask/internal/worker"
)

var (
	cfgFile     string
	databaseURL string
	schema      string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "kongtask",
	Short: "A Go implementation of graphile-worker v0.1.0",
	Long: `kongtask is a Go implementation of the original graphile-worker v0.1.0,
providing background job processing for PostgreSQL databases.

This implementation maintains strict compatibility with the original TypeScript
version's schema and behavior.`,
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

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.kongtask.yaml)")
	rootCmd.PersistentFlags().StringVar(&databaseURL, "database-url", "", "PostgreSQL connection URL")
	rootCmd.PersistentFlags().StringVar(&schema, "schema", "graphile_worker", "Schema name for worker tables")

	// Bind flags to viper
	viper.BindPFlag("database_url", rootCmd.PersistentFlags().Lookup("database-url"))
	viper.BindPFlag("schema", rootCmd.PersistentFlags().Lookup("schema"))

	// Add subcommands
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(workerCmd)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".kongtask" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".kongtask")
	}

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
	if dbURL == "" {
		return fmt.Errorf("database URL is required (use --database-url flag or DATABASE_URL env var)")
	}

	schemaName := viper.GetString("schema")
	if schemaName == "" {
		schemaName = "graphile_worker"
	}

	// Create connection pool
	pool, err := pgxpool.New(ctx, dbURL)
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
	if dbURL == "" {
		return fmt.Errorf("database URL is required (use --database-url flag or DATABASE_URL env var)")
	}

	schemaName := viper.GetString("schema")
	if schemaName == "" {
		schemaName = "graphile_worker"
	}

	// Create connection pool
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}
	defer pool.Close()

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Create worker
	w := worker.NewWorker(pool, schemaName)

	// Register example task handler
	w.RegisterTask("example_task", func(ctx context.Context, job *worker.Job) error {
		log.Printf("Processing example task with payload: %s", string(job.Payload))
		return nil
	})

	log.Printf("Starting worker with schema: %s", schemaName)

	// Run worker
	if err := w.Run(ctx); err != nil && err != context.Canceled {
		return fmt.Errorf("worker failed: %w", err)
	}

	log.Println("Worker stopped gracefully")
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
