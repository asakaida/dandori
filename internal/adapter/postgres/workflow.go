package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/google/uuid"
)

type WorkflowStore struct {
	store *Store
}

func (s *WorkflowStore) Create(ctx context.Context, wf domain.WorkflowExecution) error {
	res, err := s.store.conn(ctx).ExecContext(ctx,
		`INSERT INTO workflow_executions (id, workflow_type, task_queue, status, input, result, error_message, closed_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NULL, NULL, NULL, NOW())
		 ON CONFLICT (id) DO UPDATE SET
			workflow_type = EXCLUDED.workflow_type,
			task_queue = EXCLUDED.task_queue,
			status = EXCLUDED.status,
			input = EXCLUDED.input,
			result = NULL,
			error_message = NULL,
			closed_at = NULL,
			updated_at = NOW()
		 WHERE workflow_executions.status IN ('COMPLETED', 'FAILED', 'TERMINATED')`,
		wf.ID, wf.WorkflowType, wf.TaskQueue, wf.Status, wf.Input,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrWorkflowAlreadyExists
	}
	return nil
}

func (s *WorkflowStore) Get(ctx context.Context, id uuid.UUID) (*domain.WorkflowExecution, error) {
	var wf domain.WorkflowExecution
	var input, result []byte
	var errMsg sql.NullString
	var closedAt sql.NullTime
	err := s.store.conn(ctx).QueryRowContext(ctx,
		`SELECT id, workflow_type, task_queue, status, input, result, error_message, created_at, closed_at
		 FROM workflow_executions WHERE id = $1`, id,
	).Scan(
		&wf.ID, &wf.WorkflowType, &wf.TaskQueue, &wf.Status,
		&input, &result, &errMsg, &wf.CreatedAt, &closedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrWorkflowNotFound
	}
	if err != nil {
		return nil, err
	}
	wf.Input = input
	wf.Result = result
	if errMsg.Valid {
		wf.Error = errMsg.String
	}
	if closedAt.Valid {
		wf.ClosedAt = &closedAt.Time
	}
	return &wf, nil
}

func (s *WorkflowStore) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.WorkflowStatus, result json.RawMessage, errMsg string) error {
	var closedAt *time.Time
	if status.IsTerminal() {
		now := time.Now()
		closedAt = &now
	}
	_, err := s.store.conn(ctx).ExecContext(ctx,
		`UPDATE workflow_executions
		 SET status = $2, result = $3, error_message = $4, closed_at = $5, updated_at = NOW()
		 WHERE id = $1`,
		id, status, result, errMsg, closedAt,
	)
	return err
}
