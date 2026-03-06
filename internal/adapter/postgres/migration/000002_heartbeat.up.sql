ALTER TABLE activity_tasks
    ADD COLUMN heartbeat_at TIMESTAMPTZ,
    ADD COLUMN heartbeat_timeout INTERVAL;

CREATE INDEX idx_activity_tasks_heartbeat
    ON activity_tasks(heartbeat_at)
    WHERE status = 'RUNNING' AND heartbeat_timeout IS NOT NULL;
