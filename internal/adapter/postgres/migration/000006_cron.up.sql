ALTER TABLE workflow_executions
    ADD COLUMN IF NOT EXISTS cron_schedule VARCHAR(255),
    ADD COLUMN IF NOT EXISTS continued_as_new_id UUID REFERENCES workflow_executions(id);
