package worker

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/william-yangbo/kongtask/internal/migrate"
)

// Migrate executes database migrations for the worker schema
// This function mirrors the TypeScript migrate() function from migrate.ts
// and provides a direct equivalent to graphile-worker's migrate function
func Migrate(ctx context.Context, options WorkerSharedOptions, pool *pgxpool.Pool) error {
	// Extract schema from options (same logic as TypeScript processSharedOptions)
	schema := getSchemaFromOptions(options)

	// Create migrator with the schema
	migrator := migrate.NewMigrator(pool, schema)

	// Execute the migration (follows the same logic as TypeScript version)
	return migrator.Migrate(ctx)
}

// MigrateWithSharedOptions executes database migrations using SharedOptions
// This provides compatibility with different option types
func MigrateWithSharedOptions(ctx context.Context, options SharedOptions, pool *pgxpool.Pool) error {
	schema := "graphile_worker" // default
	if options.Schema != nil {
		schema = *options.Schema
	}

	// Check environment variable like graphile-worker does
	if envSchema := os.Getenv("GRAPHILE_WORKER_SCHEMA"); envSchema != "" {
		schema = envSchema
	}

	migrator := migrate.NewMigrator(pool, schema)
	return migrator.Migrate(ctx)
}

// getSchemaFromOptions extracts schema from WorkerSharedOptions
// This mimics the schema extraction logic from TypeScript processSharedOptions
func getSchemaFromOptions(options WorkerSharedOptions) string {
	// Default schema like graphile-worker
	schema := "graphile_worker"

	// Check environment variable (same as TypeScript processSharedOptions)
	if envSchema := os.Getenv("GRAPHILE_WORKER_SCHEMA"); envSchema != "" {
		schema = envSchema
	}

	return schema
}
