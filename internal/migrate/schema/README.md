# Database Schema Exports

This directory contains exported database schema files for verification and documentation purposes.

## Purpose

Schema files in this directory serve as:

- **Migration Verification**: Ensure migration files produce the expected database structure
- **Documentation**: Complete reference of database schema at different points in time
- **Version Control**: Track schema changes alongside migration files
- **Environment Validation**: Verify consistency across different environments

## File Naming Convention

- `graphile_worker.sql` - Default schema export (most common)
- `{schema_name}.sql` - Custom schema exports
- Files are auto-generated and should not be edited manually

## Usage

### Generate Schema Export

```bash
# Default schema (graphile_worker)
make db-dump

# Custom schema
make db-dump-custom SCHEMA=my_schema OUTPUT=internal/migrate/schema/my_schema.sql

# Direct tool usage
./bin/schema-dump -schema my_schema -output internal/migrate/schema/my_schema.sql
```

### Development Workflow

1. **Create Migration**: Add new migration file in `sql/`
2. **Run Migration**: Test migration on development database
3. **Export Schema**: Generate updated schema file
4. **Verify Changes**: Review the schema diff
5. **Commit Both**: Include both migration and schema files in commit

### Example Workflow

```bash
# 1. Create migration
echo "ALTER TABLE jobs ADD COLUMN priority INTEGER DEFAULT 0;" > internal/migrate/sql/000006.sql

# 2. Run migration
./bin/kongtask migrate -conn "postgres://user:pass@localhost/test_db"

# 3. Export updated schema
DATABASE_URL="postgres://user:pass@localhost/test_db" make db-dump

# 4. Review changes
git diff internal/migrate/schema/graphile_worker.sql

# 5. Commit changes
git add internal/migrate/sql/000006.sql
git add internal/migrate/schema/graphile_worker.sql
git commit -m "feat: add job priority support"
```

## Schema File Contents

Each schema file contains:

1. **Header**: Generation timestamp and warnings
2. **Schema Creation**: `CREATE SCHEMA` statements
3. **Tables**: Complete table definitions with constraints
4. **Indexes**: All non-primary key indexes
5. **Functions**: Stored procedures and triggers

## Validation

To validate that migrations produce the correct schema:

```bash
# Start fresh database
docker run -e POSTGRES_HOST_AUTH_METHOD=trust -d -p 5432:5432 postgres:16

# Run all migrations
PGUSER=postgres PGHOST=localhost ./bin/kongtask migrate

# Export schema
PGUSER=postgres PGHOST=localhost make db-dump

# Compare with committed version
git diff internal/migrate/schema/graphile_worker.sql
```

## CI/CD Integration

Schema validation can be integrated into CI pipelines:

```yaml
- name: Validate Schema
  run: |
    # Run migrations
    ./bin/kongtask migrate -conn $TEST_DATABASE_URL

    # Export current schema
    ./bin/schema-dump -conn $TEST_DATABASE_URL -output current_schema.sql

    # Compare with committed schema
    diff internal/migrate/schema/graphile_worker.sql current_schema.sql
```

## Notes

- Schema files are auto-generated - do not edit manually
- Keep schema files in version control alongside migrations
- Use schema diffs to review the impact of migrations
- Schema exports help catch unintended changes or missing migrations
