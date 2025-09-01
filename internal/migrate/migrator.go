// Package migrate provides database migration functionality for kongtask.
// This implementation aligns with graphile-worker v0.4.0 behavior.
package migrate

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed sql/*.sql
var migrationsFS embed.FS

// Migrator handles database migrations, following graphile-worker v0.4.0 behavior
type Migrator struct {
	pool   *pgxpool.Pool
	schema string
}

// MigrationInfo represents a single migration file
type MigrationInfo struct {
	Number   int
	Filename string
	Content  string
}

// NewMigrator creates a new migrator instance
func NewMigrator(pool *pgxpool.Pool, schema string) *Migrator {
	if schema == "" {
		// Use environment variable or default
		if envSchema := os.Getenv("GRAPHILE_WORKER_SCHEMA"); envSchema != "" {
			schema = envSchema
		} else {
			schema = "graphile_worker"
		}
	}
	return &Migrator{
		pool:   pool,
		schema: schema,
	}
}

// installSchema installs the base database schema (mirrors TypeScript installSchema function)
func (m *Migrator) installSchema(ctx context.Context, conn *pgxpool.Conn) error {
	// Create the schema with the configured name
	schemaSQL := fmt.Sprintf(`
		CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;
		CREATE SCHEMA %s;
		CREATE TABLE %s.migrations(
			id int PRIMARY KEY,
			ts timestamptz DEFAULT now() NOT NULL
		);
	`, m.schema, m.schema)

	_, err := conn.Exec(ctx, schemaSQL)
	if err != nil {
		return fmt.Errorf("failed to install schema: %w", err)
	}

	return nil
}

// loadMigrations loads migration files (mirrors TypeScript file scanning logic)
func (m *Migrator) loadMigrations() ([]MigrationInfo, error) {
	var migrations []MigrationInfo
	migrationPattern := regexp.MustCompile(`^(\d{6})\.sql$`)

	entries, err := fs.ReadDir(migrationsFS, "sql")
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		matches := migrationPattern.FindStringSubmatch(filename)
		if len(matches) != 2 {
			continue // Skip non-matching files
		}

		// Parse migration number
		number, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("invalid migration number in %s: %w", filename, err)
		}

		// Read file content
		content, err := fs.ReadFile(migrationsFS, "sql/"+filename)
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", filename, err)
		}

		migrations = append(migrations, MigrationInfo{
			Number:   number,
			Filename: filename,
			Content:  string(content),
		})
	}

	// Sort by migration number
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Number < migrations[j].Number
	})

	return migrations, nil
}

// getLatestMigration gets the latest applied migration version (mirrors TypeScript logic)
func (m *Migrator) getLatestMigration(ctx context.Context, conn *pgxpool.Conn) (int, bool, error) {
	var latestID *int

	err := conn.QueryRow(ctx,
		fmt.Sprintf("SELECT id FROM %s.migrations ORDER BY id DESC LIMIT 1", m.schema),
	).Scan(&latestID)

	if err != nil {
		// Check if it's a table doesn't exist error (same error handling as TypeScript)
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "42P01" {
			return 0, false, nil // Need to install base schema
		}
		return 0, false, fmt.Errorf("failed to get latest migration: %w", err)
	}

	if latestID != nil {
		return *latestID, true, nil
	}

	return 0, true, nil // Table exists but no migration records
}

// runMigration executes a single migration with schema replacement (mirrors graphile-worker v0.4.0)
func (m *Migrator) runMigration(ctx context.Context, tx pgx.Tx, migration MigrationInfo) error {
	// Replace :GRAPHILE_WORKER_SCHEMA with actual schema name (same as graphile-worker)
	migrationSQL := migration.Content

	// Use simple string replacement like graphile-worker does
	migrationSQL = regexp.MustCompile(`:GRAPHILE_WORKER_SCHEMA`).ReplaceAllString(migrationSQL, m.schema)

	// Also replace hardcoded "graphile_worker" references for backward compatibility
	migrationSQL = regexp.MustCompile(`\bgraphile_worker\.`).ReplaceAllString(migrationSQL, m.schema+".")
	migrationSQL = regexp.MustCompile(`\bgraphile_worker\b`).ReplaceAllString(migrationSQL, m.schema)

	// Execute migration SQL
	_, err := tx.Exec(ctx, migrationSQL)
	if err != nil {
		return fmt.Errorf("failed to execute migration %s: %w", migration.Filename, err)
	} // Record migration (v0.4.0 still uses simple id/ts structure)
	_, err = tx.Exec(ctx,
		fmt.Sprintf("INSERT INTO %s.migrations (id) VALUES ($1)", m.schema),
		migration.Number,
	)
	if err != nil {
		return fmt.Errorf("failed to record migration %s: %w", migration.Filename, err)
	}

	return nil
}

// Migrate executes database migrations (follows graphile-worker v0.4.0 migrate function logic)
func (m *Migrator) Migrate(ctx context.Context) error {
	conn, err := m.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	// Get current migration state
	latestMigration, schemaExists, err := m.getLatestMigration(ctx, conn)
	if err != nil {
		return err
	}

	// Install base schema if needed (same condition as v0.4.0)
	if !schemaExists {
		if err := m.installSchema(ctx, conn); err != nil {
			return err
		}
		latestMigration = 0
	}

	// Load all migration files
	migrations, err := m.loadMigrations()
	if err != nil {
		return err
	}

	// Execute unapplied migrations (each migration in its own transaction, same as v0.4.0)
	for _, migration := range migrations {
		if migration.Number > latestMigration {
			tx, err := conn.Begin(ctx)
			if err != nil {
				return fmt.Errorf("failed to begin transaction: %w", err)
			}

			err = m.runMigration(ctx, tx, migration)
			if err != nil {
				_ = tx.Rollback(ctx) // Rollback error is not critical here
				return err
			}

			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("failed to commit migration %s: %w", migration.Filename, err)
			}
		}
	}

	return nil
}
