CREATE TABLE IF NOT EXISTS workflow_queries (
    id            BIGSERIAL PRIMARY KEY,
    workflow_id   UUID NOT NULL REFERENCES workflow_executions(id),
    query_type    TEXT NOT NULL,
    input         JSONB,
    result        JSONB,
    error_message TEXT,
    status        TEXT NOT NULL DEFAULT 'PENDING',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    answered_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_workflow_queries_pending
    ON workflow_queries(workflow_id, status)
    WHERE status = 'PENDING';
