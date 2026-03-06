-- Revert partitioned workflow_events back to a regular table.

DO $$
BEGIN
    -- Check if not partitioned (partition p0 does not exist)
    IF NOT EXISTS (
        SELECT 1 FROM pg_class WHERE relname = 'workflow_events_p0'
    ) THEN
        RAISE NOTICE 'workflow_events is not partitioned, skipping';
        RETURN;
    END IF;

    -- 1. Rename partitioned table
    ALTER TABLE workflow_events RENAME TO workflow_events_partitioned;

    -- 2. Create regular table
    CREATE TABLE workflow_events (
        id           BIGSERIAL PRIMARY KEY,
        workflow_id  UUID NOT NULL REFERENCES workflow_executions(id),
        sequence_num INT NOT NULL,
        event_type   TEXT NOT NULL,
        event_data   JSONB NOT NULL,
        timestamp    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE (workflow_id, sequence_num)
    );

    -- 3. Migrate data back
    INSERT INTO workflow_events (id, workflow_id, sequence_num, event_type, event_data, timestamp)
    SELECT id, workflow_id, sequence_num, event_type, event_data, timestamp
    FROM workflow_events_partitioned;

    -- 4. Reset sequence
    SELECT setval('workflow_events_id_seq', COALESCE((SELECT MAX(id) FROM workflow_events), 0));

    -- 5. Drop partitioned table (cascades to all partitions)
    DROP TABLE workflow_events_partitioned;

    -- 6. Create index
    CREATE INDEX idx_workflow_events_workflow_id ON workflow_events (workflow_id);
END $$;
