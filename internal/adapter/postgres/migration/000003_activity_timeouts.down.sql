DROP INDEX IF EXISTS idx_activity_tasks_schedule_to_start_timeout;
DROP INDEX IF EXISTS idx_activity_tasks_schedule_to_close_timeout;
ALTER TABLE activity_tasks
    DROP COLUMN IF EXISTS schedule_to_start_timeout_at,
    DROP COLUMN IF EXISTS schedule_to_start_timeout,
    DROP COLUMN IF EXISTS schedule_to_close_timeout_at,
    DROP COLUMN IF EXISTS schedule_to_close_timeout;
