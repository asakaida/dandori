DROP INDEX IF EXISTS idx_workflow_executions_ns;
DROP INDEX IF EXISTS idx_activity_tasks_poll_ns;
DROP INDEX IF EXISTS idx_workflow_tasks_poll_ns;

ALTER TABLE timers DROP COLUMN IF EXISTS namespace;
ALTER TABLE activity_tasks DROP COLUMN IF EXISTS namespace;
ALTER TABLE workflow_tasks DROP COLUMN IF EXISTS namespace;
ALTER TABLE workflow_executions DROP COLUMN IF EXISTS namespace;

DROP TABLE IF EXISTS namespaces;
