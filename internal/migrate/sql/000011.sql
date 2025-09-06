-- Drop complete_job, fail_job, and get_job functions (moved to inline SQL in application code)
-- This migration corresponds to graphile-worker commits 5ad2889, ea41402, and e7dd7d8
drop function if exists :GRAPHILE_WORKER_SCHEMA.complete_job(text, bigint);
drop function if exists :GRAPHILE_WORKER_SCHEMA.fail_job(text, bigint, text);
drop function if exists :GRAPHILE_WORKER_SCHEMA.get_job(text, text[], interval, text[], timestamptz);
