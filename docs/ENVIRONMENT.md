# Environment Variables

KongTask supports comprehensive environment variable configuration with both legacy `GRAPHILE_WORKER_*` and new `KONGTASK_*` prefixes for better control and compatibility.

## Variable Priority

When multiple environment variables are available for the same setting, KongTask follows this priority order:

1. **Command line flags** (highest priority)
2. **KONGTASK\_\*** environment variables
3. **GRAPHILE*WORKER*\*** environment variables (legacy compatibility)
4. **Default values** (lowest priority)

## Supported Environment Variables

### Database Configuration

- **DATABASE_URL**: Complete PostgreSQL connection string
- **PG\*** variables\*\*: Standard PostgreSQL environment variables (PGHOST, PGPORT, PGDATABASE, PGUSER, PGPASSWORD, etc.)

### Schema Configuration

- **KONGTASK_SCHEMA** / **GRAPHILE_WORKER_SCHEMA**: Database schema name (default: `graphile_worker`)

### Worker Configuration

- **KONGTASK_CONCURRENCY** / **GRAPHILE_WORKER_CONCURRENCY**: Number of concurrent workers (default: 1)
- **KONGTASK_MAX_POOL_SIZE** / **GRAPHILE_WORKER_MAX_POOL_SIZE**: Maximum database connection pool size (default: 10)

### Logging Configuration

- **KONGTASK_DEBUG** / **GRAPHILE_WORKER_DEBUG**: Enable debug logging (default: false)
- **KONGTASK_LOG_LEVEL** / **GRAPHILE_WORKER_LOG_LEVEL**: Set logging level (debug, info, warning, error)

## Examples

### Basic Setup

```bash
# Using new KONGTASK_ prefix
export KONGTASK_DEBUG=true
export KONGTASK_CONCURRENCY=4
export KONGTASK_MAX_POOL_SIZE=20
export DATABASE_URL="postgres://user:password@localhost/kongtask_dev"

./kongtask worker
```

### Legacy Compatibility

```bash
# Using legacy GRAPHILE_WORKER_ prefix (still supported)
export GRAPHILE_WORKER_SCHEMA=custom_schema
export GRAPHILE_WORKER_DEBUG=1
export GRAPHILE_WORKER_CONCURRENCY=8

./kongtask worker
```

### Mixed Configuration with Priority

```bash
# KONGTASK_ variables take priority over GRAPHILE_WORKER_
export GRAPHILE_WORKER_CONCURRENCY=2
export KONGTASK_CONCURRENCY=6          # This will be used (6, not 2)

# Debug can be enabled with either prefix
export KONGTASK_DEBUG=true             # Enables debug logging

./kongtask worker
```

### PostgreSQL Environment Variables

```bash
# Standard PostgreSQL environment variables
export PGHOST=localhost
export PGPORT=5432
export PGDATABASE=kongtask_production
export PGUSER=kongtask_user
export PGPASSWORD=secure_password
export PGSSL=require

./kongtask worker
```

## Boolean Values

Boolean environment variables accept the following values (case-insensitive):

- **True**: `true`, `1`, `yes`, `on`
- **False**: `false`, `0`, `no`, `off`

## Development vs Production

### Development

```bash
export KONGTASK_DEBUG=true
export KONGTASK_LOG_LEVEL=debug
export DATABASE_URL="postgres://localhost/kongtask_dev"
```

### Production

```bash
export KONGTASK_DEBUG=false
export KONGTASK_LOG_LEVEL=info
export KONGTASK_CONCURRENCY=16
export KONGTASK_MAX_POOL_SIZE=50
export DATABASE_URL="postgres://user:pass@prod-host:5432/kongtask_prod?ssl=require"
```

## Migration from graphile-worker

If you're migrating from graphile-worker, your existing `GRAPHILE_WORKER_*` environment variables will continue to work. You can gradually migrate to `KONGTASK_*` prefixes for better clarity and future compatibility.

```bash
# Old (still works)
export GRAPHILE_WORKER_SCHEMA=my_schema
export GRAPHILE_WORKER_DEBUG=1

# New (recommended)
export KONGTASK_SCHEMA=my_schema
export KONGTASK_DEBUG=true
```

## Environment Variable Validation

KongTask performs intelligent validation of environment variables:

- **Database connectivity**: Validates PostgreSQL connection parameters
- **Integer values**: Ensures concurrency and pool size are positive integers
- **Boolean values**: Accepts multiple boolean formats for flexibility
- **Fallback handling**: Gracefully falls back to defaults for invalid values

## Debugging Environment Configuration

To debug your environment variable configuration, enable debug logging:

```bash
export KONGTASK_DEBUG=true
./kongtask worker
```

This will show which environment variables are being used and their resolved values during startup.
