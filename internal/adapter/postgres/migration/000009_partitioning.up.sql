-- Migrate workflow_events to hash-partitioned table (16 partitions by workflow_id).
-- This migration is idempotent: if partitions already exist, it skips.

DO $$
BEGIN
    -- Check if already partitioned (partition p0 exists)
    IF EXISTS (
        SELECT 1 FROM pg_class WHERE relname = 'workflow_events_p0'
    ) THEN
        RAISE NOTICE 'workflow_events is already partitioned, skipping';
        RETURN;
    END IF;

    -- 1. Rename original table
    ALTER TABLE workflow_events RENAME TO workflow_events_old;

    -- 2. Drop indexes on old table (they'll be recreated on the new table)
    DROP INDEX IF EXISTS idx_workflow_events_workflow_id;

    -- 3. Create new partitioned table (same structure, no BIGSERIAL — use BIGINT + sequence)
    CREATE SEQUENCE IF NOT EXISTS workflow_events_id_seq;
    PERFORM setval('workflow_events_id_seq', GREATEST(COALESCE((SELECT MAX(id) FROM workflow_events_old), 0), 1));

    CREATE TABLE workflow_events (
        id           BIGINT NOT NULL DEFAULT nextval('workflow_events_id_seq'),
        workflow_id  UUID NOT NULL,
        sequence_num INT NOT NULL,
        event_type   TEXT NOT NULL,
        event_data   JSONB NOT NULL,
        timestamp    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE (workflow_id, sequence_num)
    ) PARTITION BY HASH (workflow_id);

    ALTER SEQUENCE workflow_events_id_seq OWNED BY workflow_events.id;

    -- 4. Create 16 hash partitions
    CREATE TABLE workflow_events_p0  PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 0);
    CREATE TABLE workflow_events_p1  PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 1);
    CREATE TABLE workflow_events_p2  PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 2);
    CREATE TABLE workflow_events_p3  PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 3);
    CREATE TABLE workflow_events_p4  PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 4);
    CREATE TABLE workflow_events_p5  PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 5);
    CREATE TABLE workflow_events_p6  PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 6);
    CREATE TABLE workflow_events_p7  PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 7);
    CREATE TABLE workflow_events_p8  PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 8);
    CREATE TABLE workflow_events_p9  PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 9);
    CREATE TABLE workflow_events_p10 PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 10);
    CREATE TABLE workflow_events_p11 PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 11);
    CREATE TABLE workflow_events_p12 PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 12);
    CREATE TABLE workflow_events_p13 PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 13);
    CREATE TABLE workflow_events_p14 PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 14);
    CREATE TABLE workflow_events_p15 PARTITION OF workflow_events FOR VALUES WITH (MODULUS 16, REMAINDER 15);

    -- 5. Migrate existing data
    INSERT INTO workflow_events (id, workflow_id, sequence_num, event_type, event_data, timestamp)
    SELECT id, workflow_id, sequence_num, event_type, event_data, timestamp
    FROM workflow_events_old;

    -- 6. Re-add foreign key references from other tables
    -- (Partitioned tables in PostgreSQL support unique constraints that include the partition key,
    --  but do not support being referenced by foreign keys. The FK from the old table was implicitly
    --  dropped when we renamed it. We rely on application-level integrity instead.)

    -- 7. Drop old table
    DROP TABLE workflow_events_old;

    -- 8. Create index for partition pruning
    CREATE INDEX idx_workflow_events_workflow_id ON workflow_events (workflow_id);
END $$;
