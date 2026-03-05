CREATE TABLE workflow_executions (
    id              UUID PRIMARY KEY,
    workflow_type   TEXT NOT NULL,
    task_queue      TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'RUNNING',
    input           JSONB,
    result          JSONB,
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at       TIMESTAMPTZ
);

CREATE INDEX idx_workflow_executions_status ON workflow_executions(status);

CREATE TABLE workflow_events (
    id              BIGSERIAL PRIMARY KEY,
    workflow_id     UUID NOT NULL REFERENCES workflow_executions(id),
    sequence_num    INT NOT NULL,
    event_type      TEXT NOT NULL,
    event_data      JSONB NOT NULL,
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workflow_id, sequence_num)
);

CREATE INDEX idx_workflow_events_workflow_id ON workflow_events(workflow_id);

CREATE TABLE workflow_tasks (
    id              BIGSERIAL PRIMARY KEY,
    queue_name      TEXT NOT NULL,
    workflow_id     UUID NOT NULL REFERENCES workflow_executions(id),
    status          TEXT NOT NULL DEFAULT 'PENDING',
    scheduled_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at      TIMESTAMPTZ,
    locked_by       TEXT,
    locked_until    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workflow_tasks_poll
    ON workflow_tasks(queue_name, status, scheduled_at)
    WHERE status = 'PENDING';

CREATE TABLE activity_tasks (
    id                     BIGSERIAL PRIMARY KEY,
    queue_name             TEXT NOT NULL,
    workflow_id            UUID NOT NULL REFERENCES workflow_executions(id),
    activity_type          TEXT NOT NULL,
    activity_input         JSONB,
    activity_seq_id        BIGINT NOT NULL,
    start_to_close_timeout INTERVAL,
    retry_policy           JSONB,
    attempt                INT NOT NULL DEFAULT 1,
    max_attempts           INT NOT NULL DEFAULT 3,
    status                 TEXT NOT NULL DEFAULT 'PENDING',
    scheduled_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at             TIMESTAMPTZ,
    locked_by              TEXT,
    locked_until           TIMESTAMPTZ,
    timeout_at             TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_activity_tasks_poll
    ON activity_tasks(queue_name, status, scheduled_at)
    WHERE status = 'PENDING';

CREATE INDEX idx_activity_tasks_timeout
    ON activity_tasks(timeout_at)
    WHERE status = 'RUNNING' AND timeout_at IS NOT NULL;

CREATE TABLE timers (
    id              BIGSERIAL PRIMARY KEY,
    workflow_id     UUID NOT NULL REFERENCES workflow_executions(id),
    seq_id          BIGINT NOT NULL,
    fire_at         TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'PENDING',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_timers_pending ON timers(fire_at) WHERE status = 'PENDING';
