-- Performance optimization: extend index with locked_at and make partial
-- This migration aligns with graphile-worker commit 6067171
-- Benefits: Improved job query performance and reduced index size

-- Create new optimized index with locked_at field and partial condition
create index jobs_priority_run_at_id_locked_at_without_failures_idx
  on :GRAPHILE_WORKER_SCHEMA.jobs (priority, run_at, id, locked_at)
  where attempts < max_attempts;

-- Drop the old index as it's superseded by the new one
drop index :GRAPHILE_WORKER_SCHEMA.jobs_priority_run_at_id_idx;
