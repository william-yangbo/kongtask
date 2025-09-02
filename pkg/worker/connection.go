package worker

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

// getConnectionString resolves the database connection string using the priority order:
// 1. Provided connectionString parameter
// 2. DATABASE_URL environment variable
// 3. PostgreSQL standard environment variables (PGDATABASE must be set)
func getConnectionString(connectionString string) (string, error) {
	// Priority 1: Explicit connection string
	if connectionString != "" {
		return connectionString, nil
	}

	// Priority 2: DATABASE_URL environment variable
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		return databaseURL, nil
	}

	// Priority 3: PostgreSQL standard environment variables
	// Check if PGDATABASE is set (minimum requirement)
	if pgDatabase := os.Getenv("PGDATABASE"); pgDatabase != "" {
		// When using PG* envvars, let pgxpool.New handle the parsing automatically
		// by providing an empty connection string - pgx will read the envvars
		return "", nil
	}

	return "", fmt.Errorf("you must either specify a connection string, or you must make the DATABASE_URL or PG* environmental variables available (including at least PGDATABASE)")
}

// createDatabasePool creates a database connection pool using the resolved connection string
func createDatabasePool(ctx context.Context, connectionString string) (*pgxpool.Pool, error) {
	connString, err := getConnectionString(connectionString)
	if err != nil {
		return nil, err
	}

	// If we have an empty connection string, it means we should use PG* envvars
	// In this case, pgxpool.New with empty string will automatically read envvars
	if connString == "" {
		// Let pgx automatically read from PG* environment variables
		return pgxpool.New(ctx, "")
	}

	return pgxpool.New(ctx, connString)
}
