# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added

- **Enhanced Error Handling**: Implemented unified error handling and panic recovery mechanism (sync from graphile-worker commit 79f2160)
  - Added panic recovery for all worker pool goroutines (workers, nudge coordinator, notification handler)
  - Implemented critical error monitoring system with centralized error aggregation
  - Enhanced `Wait()` method to return critical errors that caused shutdown
  - Added `WaitWithoutError()` method for backward compatibility
  - Added `PoolError` event type for general pool error notifications
  - Improved error classification to distinguish between critical and non-critical errors
  - Added comprehensive test coverage for error handling scenarios

### Documentation

- **Improved**: Clarified `MaxAttempts` field documentation in API reference (sync from graphile-worker commit 4064051)
  - Changed terminology from "retries" to "attempts" for better accuracy
  - Added clarification that minimum value is 1, meaning task will only be attempted once and won't be retried
  - Maintains default value of 25 attempts

## [0.13.0] - 2025-09-05

### Changed

- **BREAKING**: Removed dependency on `pgcrypto` PostgreSQL extension (sync from graphile-worker v0.13.0)
  - The `jobs.queue_name` column no longer has a default value
  - If you have a pre-existing installation and wish to uninstall `pgcrypto`, you can do so manually by running `DROP EXTENSION pgcrypto;` _after_ updating to the latest schema
  - This change reduces external dependencies and aligns with modern PostgreSQL best practices
  - Migration 000010 handles the schema changes automatically

### Technical Details

- Updated migration files to remove `gen_random_uuid()` usage
- Modified `add_job` function signatures to accept `null` queue_name instead of generated UUID
- Removed `pgcrypto` extension installation from schema setup
- Updated tests to expect 10 migrations instead of 9

## Previous Versions

Previous changes were not documented in this changelog format.
