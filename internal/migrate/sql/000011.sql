-- Drop complete_job function (moved to inline SQL in application code)
-- This migration corresponds to graphile-worker commit 5ad2889
drop function if exists :GRAPHILE_WORKER_SCHEMA.complete_job(text, bigint);
