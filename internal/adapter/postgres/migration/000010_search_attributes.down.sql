DROP INDEX IF EXISTS idx_workflow_executions_search_attributes;
ALTER TABLE workflow_executions DROP COLUMN IF EXISTS search_attributes;
