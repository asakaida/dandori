ALTER TABLE activity_tasks
    ADD COLUMN schedule_to_close_timeout INTERVAL,
    ADD COLUMN schedule_to_close_timeout_at TIMESTAMPTZ,
    ADD COLUMN schedule_to_start_timeout INTERVAL,
    ADD COLUMN schedule_to_start_timeout_at TIMESTAMPTZ;

CREATE INDEX idx_activity_tasks_schedule_to_close_timeout
    ON activity_tasks(schedule_to_close_timeout_at)
    WHERE schedule_to_close_timeout_at IS NOT NULL;

CREATE INDEX idx_activity_tasks_schedule_to_start_timeout
    ON activity_tasks(schedule_to_start_timeout_at)
    WHERE status = 'PENDING' AND schedule_to_start_timeout_at IS NOT NULL;
