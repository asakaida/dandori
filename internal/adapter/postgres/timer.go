package postgres

import (
	"context"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/google/uuid"
)

type TimerStore struct {
	store *Store
}

func (s *TimerStore) Create(ctx context.Context, timer domain.Timer) error {
	_, err := s.store.conn(ctx).ExecContext(ctx,
		`INSERT INTO timers (workflow_id, seq_id, fire_at, status)
		 VALUES ($1, $2, $3, 'PENDING')`,
		timer.WorkflowID, timer.SeqID, timer.FireAt,
	)
	return err
}

func (s *TimerStore) GetFired(ctx context.Context) ([]domain.Timer, error) {
	rows, err := s.store.conn(ctx).QueryContext(ctx,
		`SELECT id, workflow_id, seq_id, fire_at, status, created_at
		 FROM timers WHERE status = 'PENDING' AND fire_at <= NOW()`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var timers []domain.Timer
	for rows.Next() {
		var t domain.Timer
		if err := rows.Scan(&t.ID, &t.WorkflowID, &t.SeqID, &t.FireAt, &t.Status, &t.CreatedAt); err != nil {
			return nil, err
		}
		timers = append(timers, t)
	}
	return timers, rows.Err()
}

func (s *TimerStore) MarkFired(ctx context.Context, timerID int64) (bool, error) {
	result, err := s.store.conn(ctx).ExecContext(ctx,
		`UPDATE timers SET status = 'FIRED' WHERE id = $1 AND status = 'PENDING'`,
		timerID,
	)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *TimerStore) Cancel(ctx context.Context, workflowID uuid.UUID, seqID int64) (bool, error) {
	result, err := s.store.conn(ctx).ExecContext(ctx,
		`UPDATE timers SET status = 'CANCELED' WHERE workflow_id = $1 AND seq_id = $2 AND status = 'PENDING'`,
		workflowID, seqID,
	)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *TimerStore) DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error {
	_, err := s.store.conn(ctx).ExecContext(ctx,
		`DELETE FROM timers WHERE workflow_id = $1`, workflowID,
	)
	return err
}
