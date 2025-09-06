-- Drop complete_job and fail_job functions (moved to inline SQL in application code)
-- This migration corresponds to graphile-worker commits 5ad2889 and ea41402
drop function if exists :GRAPHILE_WORKER_SCHEMA.complete_job(text, bigint);
drop function if exists :GRAPHILE_WORKER_SCHEMA.fail_job(text, bigint, text);
