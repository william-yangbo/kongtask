// Package main provides a tool to dump the database schema for kongtask.
// This tool exports the complete schema structure to a SQL file for verification
// and documentation purposes, similar to graphile-worker's yarn db:dump command.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SchemaDumper handles database schema export
type SchemaDumper struct {
	pool       *pgxpool.Pool
	schema     string
	outputFile string
}

// NewSchemaDumper creates a new schema dumper
func NewSchemaDumper(pool *pgxpool.Pool, schema, outputFile string) *SchemaDumper {
	if schema == "" {
		// Use environment variable or default
		if envSchema := os.Getenv("GRAPHILE_WORKER_SCHEMA"); envSchema != "" {
			schema = envSchema
		} else {
			schema = "graphile_worker"
		}
	}

	if outputFile == "" {
		// Default to internal/migrate/schema/ directory
		if schema == "graphile_worker" {
			outputFile = "internal/migrate/schema/graphile_worker.sql"
		} else {
			outputFile = fmt.Sprintf("internal/migrate/schema/%s.sql", schema)
		}
	}

	return &SchemaDumper{
		pool:       pool,
		schema:     schema,
		outputFile: outputFile,
	}
}

// DumpSchema exports the complete schema to a SQL file
func (sd *SchemaDumper) DumpSchema(ctx context.Context) error {
	var content strings.Builder

	// Header
	content.WriteString("-- KongTask Schema Dump\n")
	content.WriteString(fmt.Sprintf("-- Schema: %s\n", sd.schema))
	content.WriteString(fmt.Sprintf("-- Generated: %s\n", time.Now().Format(time.RFC3339)))
	content.WriteString("-- WARNING: This file is auto-generated. Do not edit manually.\n\n")

	// Create schema if not exists
	content.WriteString(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s;\n\n", sd.schema))

	// Export tables
	if err := sd.dumpTables(ctx, &content); err != nil {
		return fmt.Errorf("failed to dump tables: %w", err)
	}

	// Export indexes
	if err := sd.dumpIndexes(ctx, &content); err != nil {
		return fmt.Errorf("failed to dump indexes: %w", err)
	}

	// Export functions
	if err := sd.dumpFunctions(ctx, &content); err != nil {
		return fmt.Errorf("failed to dump functions: %w", err)
	}

	// Write to file
	return sd.writeToFile(content.String())
}

// dumpTables exports all tables in the schema
func (sd *SchemaDumper) dumpTables(ctx context.Context, content *strings.Builder) error {
	content.WriteString("-- Tables\n")

	query := `
		SELECT 
			table_name,
			pg_get_tabledef(schemaname||'.'||tablename) as table_def
		FROM pg_tables 
		WHERE schemaname = $1 
		ORDER BY table_name`

	rows, err := sd.pool.Query(ctx, query, sd.schema)
	if err != nil {
		// Fallback to manual table definition if pg_get_tabledef is not available
		return sd.dumpTablesManual(ctx, content)
	}
	defer rows.Close()

	for rows.Next() {
		var tableName, tableDef string
		if err := rows.Scan(&tableName, &tableDef); err != nil {
			return err
		}

		fmt.Fprintf(content, "\n-- Table: %s\n", tableName)
		content.WriteString(tableDef)
		content.WriteString(";\n")
	}

	return rows.Err()
}

// dumpTablesManual provides fallback table dumping
func (sd *SchemaDumper) dumpTablesManual(ctx context.Context, content *strings.Builder) error {
	// Get table names
	query := `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = $1 
		ORDER BY table_name`

	rows, err := sd.pool.Query(ctx, query, sd.schema)
	if err != nil {
		return err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return err
		}
		tables = append(tables, tableName)
	}

	// Dump each table
	for _, table := range tables {
		if err := sd.dumpSingleTable(ctx, content, table); err != nil {
			return err
		}
	}

	return nil
}

// dumpSingleTable exports a single table definition
func (sd *SchemaDumper) dumpSingleTable(ctx context.Context, content *strings.Builder, tableName string) error {
	fmt.Fprintf(content, "\n-- Table: %s\n", tableName)
	fmt.Fprintf(content, "CREATE TABLE %s.%s (\n", sd.schema, tableName)

	// Get columns
	query := `
		SELECT 
			column_name,
			data_type,
			is_nullable,
			column_default,
			character_maximum_length,
			numeric_precision,
			numeric_scale
		FROM information_schema.columns 
		WHERE table_schema = $1 AND table_name = $2 
		ORDER BY ordinal_position`

	rows, err := sd.pool.Query(ctx, query, sd.schema, tableName)
	if err != nil {
		return err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var columnName, dataType, isNullable string
		var columnDefault, charMaxLength, numericPrecision, numericScale *string

		if err := rows.Scan(&columnName, &dataType, &isNullable, &columnDefault, &charMaxLength, &numericPrecision, &numericScale); err != nil {
			return err
		}

		columnDef := fmt.Sprintf("    %s %s", columnName, dataType)

		// Add length/precision
		if charMaxLength != nil {
			columnDef += fmt.Sprintf("(%s)", *charMaxLength)
		} else if numericPrecision != nil {
			if numericScale != nil && *numericScale != "0" {
				columnDef += fmt.Sprintf("(%s,%s)", *numericPrecision, *numericScale)
			} else {
				columnDef += fmt.Sprintf("(%s)", *numericPrecision)
			}
		}

		// Add NOT NULL
		if isNullable == "NO" {
			columnDef += " NOT NULL"
		}

		// Add default
		if columnDefault != nil {
			columnDef += fmt.Sprintf(" DEFAULT %s", *columnDefault)
		}

		columns = append(columns, columnDef)
	}

	content.WriteString(strings.Join(columns, ",\n"))
	content.WriteString("\n);\n")

	return rows.Err()
}

// dumpIndexes exports all indexes
func (sd *SchemaDumper) dumpIndexes(ctx context.Context, content *strings.Builder) error {
	content.WriteString("\n-- Indexes\n")

	query := `
		SELECT 
			indexname,
			indexdef
		FROM pg_indexes 
		WHERE schemaname = $1 
		AND indexname NOT LIKE '%_pkey'
		ORDER BY indexname`

	rows, err := sd.pool.Query(ctx, query, sd.schema)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var indexName, indexDef string
		if err := rows.Scan(&indexName, &indexDef); err != nil {
			return err
		}

		fmt.Fprintf(content, "\n-- Index: %s\n", indexName)
		content.WriteString(indexDef)
		content.WriteString(";\n")
	}

	return rows.Err()
}

// dumpFunctions exports all functions
func (sd *SchemaDumper) dumpFunctions(ctx context.Context, content *strings.Builder) error {
	content.WriteString("\n-- Functions\n")

	query := `
		SELECT 
			routine_name,
			pg_get_functiondef(p.oid) as function_def
		FROM information_schema.routines r
		JOIN pg_proc p ON p.proname = r.routine_name
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE r.routine_schema = $1
		AND n.nspname = $1
		ORDER BY routine_name`

	rows, err := sd.pool.Query(ctx, query, sd.schema)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var functionName, functionDef string
		if err := rows.Scan(&functionName, &functionDef); err != nil {
			return err
		}

		fmt.Fprintf(content, "\n-- Function: %s\n", functionName)
		content.WriteString(functionDef)
		content.WriteString(";\n")
	}

	return rows.Err()
}

// writeToFile writes content to the output file
func (sd *SchemaDumper) writeToFile(content string) error {
	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(sd.outputFile), 0755); err != nil {
		return err
	}

	// Write file
	return os.WriteFile(sd.outputFile, []byte(content), 0644)
}

func main() {
	var (
		connStr    = flag.String("conn", "", "PostgreSQL connection string (default: from DATABASE_URL or PG* env vars)")
		schema     = flag.String("schema", "", "Schema name (default: from GRAPHILE_WORKER_SCHEMA or 'graphile_worker')")
		outputFile = flag.String("output", "", "Output file path (default: internal/migrate/schema/<schema>.sql)")
		help       = flag.Bool("help", false, "Show help message")
	)
	flag.Parse()

	if *help {
		fmt.Printf("Usage: %s [options]\n\n", os.Args[0])
		fmt.Println("Export database schema to SQL file for verification and documentation.")
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		fmt.Println("\nEnvironment Variables:")
		fmt.Println("  DATABASE_URL           - PostgreSQL connection string")
		fmt.Println("  PGHOST, PGPORT, etc.   - Standard PostgreSQL environment variables")
		fmt.Println("  GRAPHILE_WORKER_SCHEMA - Schema name (default: graphile_worker)")
		fmt.Println("\nExamples:")
		fmt.Println("  schema-dump")
		fmt.Println("  schema-dump -schema custom_schema -output internal/migrate/schema/custom.sql")
		fmt.Println("  DATABASE_URL=postgres://user:pass@localhost/db schema-dump")
		return
	}

	// Get connection string
	connectionString := *connStr
	if connectionString == "" {
		if connStr := os.Getenv("DATABASE_URL"); connStr != "" {
			connectionString = connStr
		} else {
			// Try to build from PG* environment variables
			host := os.Getenv("PGHOST")
			port := os.Getenv("PGPORT")
			user := os.Getenv("PGUSER")
			password := os.Getenv("PGPASSWORD")
			dbname := os.Getenv("PGDATABASE")

			if host == "" {
				host = "localhost"
			}
			if port == "" {
				port = "5432"
			}
			if user == "" {
				user = "postgres"
			}
			if dbname == "" {
				dbname = "postgres"
			}

			if password != "" {
				connectionString = fmt.Sprintf("postgres://%s:%s@%s:%s/%s", user, password, host, port, dbname)
			} else {
				connectionString = fmt.Sprintf("postgres://%s@%s:%s/%s", user, host, port, dbname)
			}
		}
	}

	if connectionString == "" {
		log.Fatal("No database connection string provided. Use -conn flag, DATABASE_URL, or PG* environment variables.")
	}

	// Connect to database
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connectionString)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	// Create dumper and export schema
	dumper := NewSchemaDumper(pool, *schema, *outputFile)

	fmt.Printf("Exporting schema '%s' to '%s'...\n", dumper.schema, dumper.outputFile)

	if err := dumper.DumpSchema(ctx); err != nil {
		log.Fatalf("Failed to dump schema: %v", err)
	}

	fmt.Printf("Schema exported successfully to '%s'\n", dumper.outputFile)
}
