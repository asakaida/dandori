-- namespaces テーブル
CREATE TABLE IF NOT EXISTS namespaces (
    name VARCHAR(255) PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO namespaces (name, description) VALUES ('default', 'Default namespace') ON CONFLICT DO NOTHING;

-- 各テーブルにnamespaceカラム追加
ALTER TABLE workflow_executions ADD COLUMN IF NOT EXISTS namespace VARCHAR(255) NOT NULL DEFAULT 'default' REFERENCES namespaces(name);
ALTER TABLE workflow_tasks ADD COLUMN IF NOT EXISTS namespace VARCHAR(255) NOT NULL DEFAULT 'default' REFERENCES namespaces(name);
ALTER TABLE activity_tasks ADD COLUMN IF NOT EXISTS namespace VARCHAR(255) NOT NULL DEFAULT 'default' REFERENCES namespaces(name);
ALTER TABLE timers ADD COLUMN IF NOT EXISTS namespace VARCHAR(255) NOT NULL DEFAULT 'default' REFERENCES namespaces(name);

-- Pollクエリ用の複合インデックス（namespace含む）
CREATE INDEX IF NOT EXISTS idx_workflow_tasks_poll_ns ON workflow_tasks(namespace, queue_name, status, scheduled_at) WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_activity_tasks_poll_ns ON activity_tasks(namespace, queue_name, status, scheduled_at) WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_workflow_executions_ns ON workflow_executions(namespace, created_at DESC, id DESC);
