package postgres

import (
	"context"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/google/uuid"
)

type EventStore struct {
	store *Store
}

func (s *EventStore) Append(ctx context.Context, events []domain.HistoryEvent) error {
	conn := s.store.conn(ctx)
	for _, e := range events {
		_, err := conn.ExecContext(ctx,
			`INSERT INTO workflow_events (workflow_id, sequence_num, event_type, event_data)
			 VALUES ($1, (SELECT COALESCE(MAX(sequence_num), 0) + 1 FROM workflow_events WHERE workflow_id = $1), $2, $3)`,
			e.WorkflowID, e.Type, e.Data,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *EventStore) GetByWorkflowID(ctx context.Context, workflowID uuid.UUID) ([]domain.HistoryEvent, error) {
	rows, err := s.store.conn(ctx).QueryContext(ctx,
		`SELECT id, workflow_id, sequence_num, event_type, event_data, timestamp
		 FROM workflow_events WHERE workflow_id = $1 ORDER BY sequence_num ASC`, workflowID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []domain.HistoryEvent
	for rows.Next() {
		var e domain.HistoryEvent
		if err := rows.Scan(&e.ID, &e.WorkflowID, &e.SequenceNum, &e.Type, &e.Data, &e.Timestamp); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
