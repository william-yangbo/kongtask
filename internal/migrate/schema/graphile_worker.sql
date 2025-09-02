-- KongTask Schema Dump
-- Schema: graphile_worker
-- Generated: 2025-09-02T13:59:43+08:00
-- WARNING: This file is auto-generated. Do not edit manually.

CREATE SCHEMA IF NOT EXISTS graphile_worker;

-- Tables

-- Table: job_queues
CREATE TABLE graphile_worker.job_queues (
    queue_name text NOT NULL,
    job_count integer(32) NOT NULL,
    locked_at timestamp with time zone,
    locked_by text
);

-- Table: jobs
CREATE TABLE graphile_worker.jobs (
    id bigint(64) NOT NULL DEFAULT nextval('graphile_worker.jobs_id_seq'::regclass),
    queue_name text DEFAULT (public.gen_random_uuid())::text,
    task_identifier text NOT NULL,
    payload json NOT NULL DEFAULT '{}'::json,
    priority integer(32) NOT NULL DEFAULT 0,
    run_at timestamp with time zone NOT NULL DEFAULT now(),
    attempts integer(32) NOT NULL DEFAULT 0,
    max_attempts integer(32) NOT NULL DEFAULT 25,
    last_error text,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    key text,
    locked_at timestamp with time zone,
    locked_by text,
    revision integer(32) NOT NULL DEFAULT 0,
    flags jsonb
);

-- Table: migrations
CREATE TABLE graphile_worker.migrations (
    id integer(32) NOT NULL,
    ts timestamp with time zone NOT NULL DEFAULT now()
);

-- Indexes

-- Index: jobs_key_key
CREATE UNIQUE INDEX jobs_key_key ON graphile_worker.jobs USING btree (key);

-- Index: jobs_priority_run_at_id_locked_at_without_failures_idx
CREATE INDEX jobs_priority_run_at_id_locked_at_without_failures_idx ON graphile_worker.jobs USING btree (priority, run_at, id, locked_at) WHERE (attempts < max_attempts);

-- Functions

-- Function: add_job
CREATE OR REPLACE FUNCTION graphile_worker.add_job(identifier text, payload json DEFAULT NULL::json, queue_name text DEFAULT NULL::text, run_at timestamp with time zone DEFAULT NULL::timestamp with time zone, max_attempts integer DEFAULT NULL::integer, job_key text DEFAULT NULL::text, priority integer DEFAULT NULL::integer, flags text[] DEFAULT NULL::text[])
 RETURNS graphile_worker.jobs
 LANGUAGE plpgsql
AS $function$
declare
  v_job graphile_worker.jobs;
begin
  -- Apply rationality checks
  if length(identifier) > 128 then
    raise exception 'Task identifier is too long (max length: 128).' using errcode = 'GWBID';
  end if;
  if queue_name is not null and length(queue_name) > 128 then
    raise exception 'Job queue name is too long (max length: 128).' using errcode = 'GWBQN';
  end if;
  if job_key is not null and length(job_key) > 512 then
    raise exception 'Job key is too long (max length: 512).' using errcode = 'GWBJK';
  end if;
  if max_attempts < 1 then
    raise exception 'Job maximum attempts must be at least 1' using errcode = 'GWBMA';
  end if;

  if job_key is not null then
    -- Upsert job
    insert into graphile_worker.jobs (
      task_identifier,
      payload,
      queue_name,
      run_at,
      max_attempts,
      key,
      priority,
      flags
    )
      values(
        identifier,
        coalesce(payload, '{}'::json),
        queue_name,
        coalesce(run_at, now()),
        coalesce(max_attempts, 25),
        job_key,
        coalesce(priority, 0),
        (
          select jsonb_object_agg(flag, true)
          from unnest(flags) as item(flag)
        )
      )
      on conflict (key) do update set
        task_identifier=excluded.task_identifier,
        payload=excluded.payload,
        queue_name=excluded.queue_name,
        max_attempts=excluded.max_attempts,
        run_at=excluded.run_at,
        priority=excluded.priority,
        revision=jobs.revision + 1,
        flags=excluded.flags,

        -- always reset error/retry state
        attempts=0,
        last_error=null
      where jobs.locked_at is null
      returning *
      into v_job;

    -- If upsert succeeded (insert or update), return early
    if not (v_job is null) then
      return v_job;
    end if;

    -- Upsert failed -> there must be an existing job that is locked. Remove
    -- existing key to allow a new one to be inserted, and prevent any
    -- subsequent retries by bumping attempts to the max allowed.
    update graphile_worker.jobs
      set
        key = null,
        attempts = jobs.max_attempts
      where key = job_key;
  end if;

  -- insert the new job. Assume no conflicts due to the update above
  insert into graphile_worker.jobs(
    task_identifier,
    payload,
    queue_name,
    run_at,
    max_attempts,
    key,
    priority,
    flags
  )
    values(
      identifier,
      coalesce(payload, '{}'::json),
      queue_name,
      coalesce(run_at, now()),
      coalesce(max_attempts, 25),
      job_key,
      coalesce(priority, 0),
      (
        select jsonb_object_agg(flag, true)
        from unnest(flags) as item(flag)
      )
    )
    returning *
    into v_job;

  return v_job;
end;
$function$
;

-- Function: complete_job
CREATE OR REPLACE FUNCTION graphile_worker.complete_job(worker_id text, job_id bigint)
 RETURNS graphile_worker.jobs
 LANGUAGE plpgsql
AS $function$
declare
  v_row graphile_worker.jobs;
begin
  delete from graphile_worker.jobs
    where id = job_id
    returning * into v_row;

  if v_row.queue_name is not null then
    update graphile_worker.job_queues
      set locked_by = null, locked_at = null
      where queue_name = v_row.queue_name and locked_by = worker_id;
  end if;

  return v_row;
end;
$function$
;

-- Function: complete_jobs
CREATE OR REPLACE FUNCTION graphile_worker.complete_jobs(job_ids bigint[])
 RETURNS SETOF graphile_worker.jobs
 LANGUAGE sql
AS $function$
  delete from graphile_worker.jobs
    where id = any(job_ids)
    and (
      locked_by is null
    or
      locked_at < NOW() - interval '4 hours'
    )
    returning *;
$function$
;

-- Function: fail_job
CREATE OR REPLACE FUNCTION graphile_worker.fail_job(worker_id text, job_id bigint, error_message text)
 RETURNS graphile_worker.jobs
 LANGUAGE plpgsql
 STRICT
AS $function$
declare
  v_row graphile_worker.jobs;
begin
  update graphile_worker.jobs
    set
      last_error = error_message,
      run_at = greatest(now(), run_at) + (exp(least(attempts, 10))::text || ' seconds')::interval,
      locked_by = null,
      locked_at = null
    where id = job_id and locked_by = worker_id
    returning * into v_row;

  if v_row.queue_name is not null then
    update graphile_worker.job_queues
      set locked_by = null, locked_at = null
      where queue_name = v_row.queue_name and locked_by = worker_id;
  end if;

  return v_row;
end;
$function$
;

-- Function: get_job
CREATE OR REPLACE FUNCTION graphile_worker.get_job(worker_id text, task_identifiers text[] DEFAULT NULL::text[], job_expiry interval DEFAULT '04:00:00'::interval, forbidden_flags text[] DEFAULT NULL::text[])
 RETURNS graphile_worker.jobs
 LANGUAGE plpgsql
AS $function$
declare
  v_job_id bigint;
  v_queue_name text;
  v_row graphile_worker.jobs;
  v_now timestamptz = now();
begin
  if worker_id is null or length(worker_id) < 10 then
    raise exception 'invalid worker id';
  end if;

  select jobs.queue_name, jobs.id into v_queue_name, v_job_id
    from graphile_worker.jobs
    where (jobs.locked_at is null or jobs.locked_at < (v_now - job_expiry))
    and (
      jobs.queue_name is null
    or
      exists (
        select 1
        from graphile_worker.job_queues
        where job_queues.queue_name = jobs.queue_name
        and (job_queues.locked_at is null or job_queues.locked_at < (v_now - job_expiry))
        for update
        skip locked
      )
    )
    and run_at <= v_now
    and attempts < max_attempts
    and (task_identifiers is null or task_identifier = any(task_identifiers))
    and (forbidden_flags is null or (flags ?| forbidden_flags) is not true)
    order by priority asc, run_at asc, id asc
    limit 1
    for update
    skip locked;

  if v_job_id is null then
    return null;
  end if;

  if v_queue_name is not null then
    update graphile_worker.job_queues
      set
        locked_by = worker_id,
        locked_at = v_now
      where job_queues.queue_name = v_queue_name;
  end if;

  update graphile_worker.jobs
    set
      attempts = attempts + 1,
      locked_by = worker_id,
      locked_at = v_now
    where id = v_job_id
    returning * into v_row;

  return v_row;
end;
$function$
;

-- Function: jobs__decrease_job_queue_count
CREATE OR REPLACE FUNCTION graphile_worker.jobs__decrease_job_queue_count()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
declare
  v_new_job_count int;
begin
  update graphile_worker.job_queues
    set job_count = job_queues.job_count - 1
    where queue_name = old.queue_name
    returning job_count into v_new_job_count;

  if v_new_job_count <= 0 then
    delete from graphile_worker.job_queues where queue_name = old.queue_name and job_count <= 0;
  end if;

  return old;
end;
$function$
;

-- Function: jobs__increase_job_queue_count
CREATE OR REPLACE FUNCTION graphile_worker.jobs__increase_job_queue_count()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
begin
  insert into graphile_worker.job_queues(queue_name, job_count)
    values(new.queue_name, 1)
    on conflict (queue_name)
    do update
    set job_count = job_queues.job_count + 1;

  return new;
end;
$function$
;

-- Function: permanently_fail_jobs
CREATE OR REPLACE FUNCTION graphile_worker.permanently_fail_jobs(job_ids bigint[], error_message text DEFAULT NULL::text)
 RETURNS SETOF graphile_worker.jobs
 LANGUAGE sql
AS $function$
  update graphile_worker.jobs
    set
      last_error = coalesce(error_message, 'Manually marked as failed'),
      attempts = max_attempts
    where id = any(job_ids)
    and (
      locked_by is null
    or
      locked_at < NOW() - interval '4 hours'
    )
    returning *;
$function$
;

-- Function: remove_job
CREATE OR REPLACE FUNCTION graphile_worker.remove_job(job_key text)
 RETURNS graphile_worker.jobs
 LANGUAGE sql
 STRICT
AS $function$
  delete from graphile_worker.jobs
    where key = job_key
    and locked_at is null
  returning *;
$function$
;

-- Function: reschedule_jobs
CREATE OR REPLACE FUNCTION graphile_worker.reschedule_jobs(job_ids bigint[], run_at timestamp with time zone DEFAULT NULL::timestamp with time zone, priority integer DEFAULT NULL::integer, attempts integer DEFAULT NULL::integer, max_attempts integer DEFAULT NULL::integer)
 RETURNS SETOF graphile_worker.jobs
 LANGUAGE sql
AS $function$
  update graphile_worker.jobs
    set
      run_at = coalesce(reschedule_jobs.run_at, jobs.run_at),
      priority = coalesce(reschedule_jobs.priority, jobs.priority),
      attempts = coalesce(reschedule_jobs.attempts, jobs.attempts),
      max_attempts = coalesce(reschedule_jobs.max_attempts, jobs.max_attempts)
    where id = any(job_ids)
    and (
      locked_by is null
    or
      locked_at < NOW() - interval '4 hours'
    )
    returning *;
$function$
;

-- Function: tg__update_timestamp
CREATE OR REPLACE FUNCTION graphile_worker.tg__update_timestamp()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
begin
  new.updated_at = greatest(now(), old.updated_at + interval '1 millisecond');
  return new;
end;
$function$
;

-- Function: tg_jobs__notify_new_jobs
CREATE OR REPLACE FUNCTION graphile_worker.tg_jobs__notify_new_jobs()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
begin
  perform pg_notify('jobs:insert', '');
  return new;
end;
$function$
;
