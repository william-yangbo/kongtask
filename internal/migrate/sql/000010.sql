-- Migration 000010: Remove dependency on pgcrypto extension (sync from graphile-worker c84f334)
-- This migration removes the pgcrypto dependency and gen_random_uuid() usage
-- Background: graphile-worker v0.13.0 removed pgcrypto to reduce external dependencies

-- Remove the default value from queue_name column
alter table :GRAPHILE_WORKER_SCHEMA.jobs alter column queue_name drop default;
