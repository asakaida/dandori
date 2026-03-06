ALTER TABLE workflow_executions
    ADD COLUMN IF NOT EXISTS parent_workflow_id UUID REFERENCES workflow_executions(id),
    ADD COLUMN IF NOT EXISTS parent_seq_id INTEGER;

CREATE INDEX IF NOT EXISTS idx_workflow_executions_parent
    ON workflow_executions(parent_workflow_id)
    WHERE parent_workflow_id IS NOT NULL;
