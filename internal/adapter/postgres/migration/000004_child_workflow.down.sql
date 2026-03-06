DROP INDEX IF EXISTS idx_workflow_executions_parent;

ALTER TABLE workflow_executions
    DROP COLUMN IF EXISTS parent_seq_id,
    DROP COLUMN IF EXISTS parent_workflow_id;
