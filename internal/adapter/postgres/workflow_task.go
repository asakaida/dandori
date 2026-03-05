package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/asakaida/dandori/internal/domain"
)

type WorkflowTaskStore struct {
	store *Store
}

func (s *WorkflowTaskStore) Enqueue(ctx context.Context, task domain.WorkflowTask) error {
	_, err := s.store.conn(ctx).ExecContext(ctx,
		`INSERT INTO workflow_tasks (queue_name, workflow_id, status, scheduled_at)
		 VALUES ($1, $2, 'PENDING', COALESCE($3, NOW()))`,
		task.QueueName, task.WorkflowID, nullTimeIfZero(task.ScheduledAt),
	)
	return err
}

func (s *WorkflowTaskStore) Poll(ctx context.Context, queueName string, workerID string) (*domain.WorkflowTask, error) {
	var task domain.WorkflowTask
	err := s.store.conn(ctx).QueryRowContext(ctx,
		`UPDATE workflow_tasks SET
			status = 'RUNNING',
			started_at = NOW(),
			locked_by = $2,
			locked_until = NOW() + INTERVAL '30 seconds'
		 WHERE id = (
			SELECT id FROM workflow_tasks
			WHERE queue_name = $1 AND status = 'PENDING' AND scheduled_at <= NOW()
			ORDER BY scheduled_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, queue_name, workflow_id, status, scheduled_at`,
		queueName, workerID,
	).Scan(&task.ID, &task.QueueName, &task.WorkflowID, &task.Status, &task.ScheduledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNoTaskAvailable
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *WorkflowTaskStore) Complete(ctx context.Context, taskID int64) error {
	res, err := s.store.conn(ctx).ExecContext(ctx,
		`UPDATE workflow_tasks SET status = 'COMPLETED' WHERE id = $1 AND status = 'RUNNING'`,
		taskID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrTaskAlreadyCompleted
	}
	return nil
}

func (s *WorkflowTaskStore) GetByID(ctx context.Context, taskID int64) (*domain.WorkflowTask, error) {
	var task domain.WorkflowTask
	err := s.store.conn(ctx).QueryRowContext(ctx,
		`SELECT id, queue_name, workflow_id, status, scheduled_at
		 FROM workflow_tasks WHERE id = $1`, taskID,
	).Scan(&task.ID, &task.QueueName, &task.WorkflowID, &task.Status, &task.ScheduledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}

	// Advisory lock for workflow serialization within transaction
	if txFromContext(ctx) != nil {
		_, err = s.store.conn(ctx).ExecContext(ctx,
			`SELECT pg_advisory_xact_lock(hashtext($1::text))`, task.WorkflowID.String(),
		)
		if err != nil {
			return nil, err
		}
	}

	return &task, nil
}

func (s *WorkflowTaskStore) RecoverStaleTasks(ctx context.Context) (int, error) {
	res, err := s.store.conn(ctx).ExecContext(ctx,
		`UPDATE workflow_tasks SET status = 'PENDING', locked_by = NULL, locked_until = NULL
		 WHERE status = 'RUNNING' AND locked_until < NOW()`,
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}
