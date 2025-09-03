-- Migration 000008: Add cron scheduling support (sync from graphile-worker cb369ad)
-- This migration adds the known_crontabs table to support distributed cron scheduling

CREATE TABLE :GRAPHILE_WORKER_SCHEMA.known_crontabs (
    identifier TEXT NOT NULL PRIMARY KEY,
    known_since TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_execution TIMESTAMPTZ
);

-- Enable row level security (consistent with other tables)
ALTER TABLE :GRAPHILE_WORKER_SCHEMA.known_crontabs ENABLE ROW LEVEL SECURITY;

-- Add index for performance
CREATE INDEX idx_known_crontabs_last_execution ON :GRAPHILE_WORKER_SCHEMA.known_crontabs(last_execution);

-- Comment for documentation
COMMENT ON TABLE :GRAPHILE_WORKER_SCHEMA.known_crontabs IS 'Tracks known cron identifiers and their execution history to prevent duplicate scheduling across multiple workers';
COMMENT ON COLUMN :GRAPHILE_WORKER_SCHEMA.known_crontabs.identifier IS 'Unique identifier for the cron item';
COMMENT ON COLUMN :GRAPHILE_WORKER_SCHEMA.known_crontabs.known_since IS 'When this cron identifier was first registered';
COMMENT ON COLUMN :GRAPHILE_WORKER_SCHEMA.known_crontabs.last_execution IS 'Timestamp of the last successful execution';
