-- Performance test initialization script
-- Creates 20,000 jobs for log_if_999 task (matching original perfTest)

DO $$
BEGIN
    -- Add jobs for performance testing
    PERFORM graphile_worker.add_job('log_if_999', json_build_object('id', i)) 
    FROM generate_series(1, 20000) i;
END;
$$ LANGUAGE plpgsql;
