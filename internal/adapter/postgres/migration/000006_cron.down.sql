ALTER TABLE workflow_executions
    DROP COLUMN IF EXISTS continued_as_new_id,
    DROP COLUMN IF EXISTS cron_schedule;
