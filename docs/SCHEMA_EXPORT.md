# Schema Export Tool

The `schema-dump` tool exports the complete database schema structure to a SQL file for verification and documentation purposes.

## Purpose

- **Migration Verification**: Ensure migration files produce the expected database structure
- **Documentation**: Provide a complete reference of the current schema
- **Environment Consistency**: Verify that different environments have the same structure
- **Debugging**: Compare actual schema with expected structure

## Usage

### Basic Usage

```bash
# Export default schema (graphile_worker) to schema_graphile_worker.sql
make db-dump

# Or directly
./bin/schema-dump
```

### Custom Schema

```bash
# Export custom schema
make db-dump-custom SCHEMA=my_schema OUTPUT=my_schema.sql

# Or directly
./bin/schema-dump -schema my_schema -output my_schema.sql
```

### Using Environment Variables

```bash
# Using DATABASE_URL
DATABASE_URL=postgres://user:pass@localhost/db ./bin/schema-dump

# Using PG* environment variables
PGHOST=localhost PGUSER=postgres PGDATABASE=mydb ./bin/schema-dump

# Custom schema via environment
GRAPHILE_WORKER_SCHEMA=custom_schema ./bin/schema-dump
```

## Output Format

The exported SQL file includes:

1. **Header Information**: Schema name, generation timestamp, warnings
2. **Schema Creation**: `CREATE SCHEMA` statement
3. **Tables**: Complete table definitions with columns, constraints, defaults
4. **Indexes**: All indexes except primary key indexes
5. **Functions**: All stored procedures and functions

## Development Workflow

### Adding New Migrations

1. Create your migration file:

   ```sql
   -- internal/migrate/sql/000006.sql
   ALTER TABLE jobs ADD COLUMN priority INTEGER DEFAULT 0;
   CREATE INDEX idx_jobs_priority ON jobs(priority);
   ```

2. Test the migration:

   ```bash
   # Run migration on test database
   ./bin/kongtask migrate -conn "postgres://user:pass@localhost/test_db"
   ```

3. Export and verify schema:

   ```bash
   # Export updated schema
   DATABASE_URL=postgres://user:pass@localhost/test_db make db-dump

   # Review the generated schema_graphile_worker.sql
   # Verify it contains your changes
   ```

4. Commit both files:
   ```bash
   git add internal/migrate/sql/000006.sql
   git add schema_graphile_worker.sql
   git commit -m "feat: add job priority support"
   ```

### Verifying Migrations

To verify that your migration files can rebuild the complete schema:

```bash
# 1. Start with a clean database
docker run -e POSTGRES_HOST_AUTH_METHOD=trust -d -p 5432:5432 postgres:16

# 2. Run all migrations
PGUSER=postgres PGHOST=localhost ./bin/kongtask migrate

# 3. Export schema
PGUSER=postgres PGHOST=localhost ./bin/schema-dump

# 4. Compare with the committed schema file
diff schema_graphile_worker.sql expected_schema.sql
```

## Troubleshooting

### Permission Issues

```bash
# Ensure the database user has sufficient permissions
GRANT USAGE ON SCHEMA information_schema TO your_user;
GRANT SELECT ON ALL TABLES IN SCHEMA information_schema TO your_user;
```

### Connection Issues

```bash
# Test connection first
PGUSER=postgres PGHOST=localhost psql -c "SELECT version();"

# Then run schema dump
PGUSER=postgres PGHOST=localhost ./bin/schema-dump
```

### Large Schemas

For schemas with many tables, the export may take some time. The tool provides progress information:

```bash
# Example output
Exporting schema 'graphile_worker' to 'schema_graphile_worker.sql'...
Schema exported successfully to 'schema_graphile_worker.sql'
```

## Integration with CI/CD

Add schema verification to your CI pipeline:

```yaml
# .github/workflows/test.yml
- name: Verify Schema
  run: |
    # Run migrations
    ./bin/kongtask migrate -conn $DATABASE_URL

    # Export schema
    ./bin/schema-dump -conn $DATABASE_URL -output current_schema.sql

    # Compare with committed schema
    diff schema_graphile_worker.sql current_schema.sql
```

This ensures that:

- Migration files are complete and correct
- No manual schema changes have been made
- The committed schema matches the actual database structure
