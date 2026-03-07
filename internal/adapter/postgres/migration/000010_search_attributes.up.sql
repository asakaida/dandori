ALTER TABLE workflow_executions ADD COLUMN search_attributes JSONB NOT NULL DEFAULT '{}';

CREATE INDEX idx_workflow_executions_search_attributes ON workflow_executions USING GIN (search_attributes);
