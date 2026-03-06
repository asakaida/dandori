DROP INDEX IF EXISTS idx_activity_tasks_heartbeat;

ALTER TABLE activity_tasks
    DROP COLUMN IF EXISTS heartbeat_at,
    DROP COLUMN IF EXISTS heartbeat_timeout;
