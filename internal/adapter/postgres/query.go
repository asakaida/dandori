package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/google/uuid"
)

type QueryStore struct {
	store *Store
}

func (s *QueryStore) Create(ctx context.Context, query domain.WorkflowQuery) (int64, error) {
	var id int64
	err := s.store.conn(ctx).QueryRowContext(ctx,
		`INSERT INTO workflow_queries (workflow_id, query_type, input, status)
		 VALUES ($1, $2, $3, 'PENDING')
		 RETURNING id`,
		query.WorkflowID, query.QueryType, jsonOrNull(query.Input),
	).Scan(&id)
	return id, err
}

func (s *QueryStore) GetByID(ctx context.Context, queryID int64) (*domain.WorkflowQuery, error) {
	var q domain.WorkflowQuery
	var input, result []byte
	var errMsg sql.NullString
	var answeredAt sql.NullTime
	err := s.store.conn(ctx).QueryRowContext(ctx,
		`SELECT id, workflow_id, query_type, input, result, error_message, status, created_at, answered_at
		 FROM workflow_queries WHERE id = $1`, queryID,
	).Scan(&q.ID, &q.WorkflowID, &q.QueryType, &input, &result, &errMsg, &q.Status, &q.CreatedAt, &answeredAt)
	if err == sql.ErrNoRows {
		return nil, domain.ErrQueryNotFound
	}
	if err != nil {
		return nil, err
	}
	if input != nil {
		q.Input = json.RawMessage(input)
	}
	if result != nil {
		q.Result = json.RawMessage(result)
	}
	if errMsg.Valid {
		q.ErrorMessage = errMsg.String
	}
	if answeredAt.Valid {
		q.AnsweredAt = &answeredAt.Time
	}
	return &q, nil
}

func (s *QueryStore) GetPendingByWorkflowID(ctx context.Context, workflowID uuid.UUID) ([]domain.WorkflowQuery, error) {
	rows, err := s.store.conn(ctx).QueryContext(ctx,
		`SELECT id, workflow_id, query_type, input, status, created_at
		 FROM workflow_queries
		 WHERE workflow_id = $1 AND status = 'PENDING'
		 ORDER BY created_at`, workflowID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queries []domain.WorkflowQuery
	for rows.Next() {
		var q domain.WorkflowQuery
		var input []byte
		if err := rows.Scan(&q.ID, &q.WorkflowID, &q.QueryType, &input, &q.Status, &q.CreatedAt); err != nil {
			return nil, err
		}
		if input != nil {
			q.Input = json.RawMessage(input)
		}
		queries = append(queries, q)
	}
	return queries, rows.Err()
}

func (s *QueryStore) SetResult(ctx context.Context, queryID int64, result json.RawMessage, errMsg string) error {
	res, err := s.store.conn(ctx).ExecContext(ctx,
		`UPDATE workflow_queries
		 SET status = 'ANSWERED', result = $2, error_message = $3, answered_at = NOW()
		 WHERE id = $1 AND status = 'PENDING'`,
		queryID, jsonOrNull(result), nullIfEmpty(errMsg),
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrQueryNotFound
	}
	return nil
}

func (s *QueryStore) DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error {
	_, err := s.store.conn(ctx).ExecContext(ctx,
		`DELETE FROM workflow_queries WHERE workflow_id = $1`, workflowID,
	)
	return err
}

func jsonOrNull(data json.RawMessage) any {
	if len(data) == 0 {
		return nil
	}
	return []byte(data)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
