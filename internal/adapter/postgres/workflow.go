package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/asakaida/dandori/internal/port"
	"github.com/google/uuid"
)

type WorkflowStore struct {
	store *Store
}

func (s *WorkflowStore) Create(ctx context.Context, wf domain.WorkflowExecution) error {
	searchAttrs := "{}"
	if len(wf.SearchAttributes) > 0 {
		b, _ := json.Marshal(wf.SearchAttributes)
		searchAttrs = string(b)
	}
	res, err := s.store.conn(ctx).ExecContext(ctx,
		`INSERT INTO workflow_executions (id, namespace, workflow_type, task_queue, status, input, result, error_message, closed_at, updated_at, parent_workflow_id, parent_seq_id, cron_schedule, search_attributes)
		 VALUES ($1, $2, $3, $4, $5, $6, NULL, NULL, NULL, NOW(), $7, $8, $9, $10)
		 ON CONFLICT (id) DO UPDATE SET
			namespace = EXCLUDED.namespace,
			workflow_type = EXCLUDED.workflow_type,
			task_queue = EXCLUDED.task_queue,
			status = EXCLUDED.status,
			input = EXCLUDED.input,
			result = NULL,
			error_message = NULL,
			closed_at = NULL,
			updated_at = NOW(),
			parent_workflow_id = EXCLUDED.parent_workflow_id,
			parent_seq_id = EXCLUDED.parent_seq_id,
			cron_schedule = EXCLUDED.cron_schedule,
			search_attributes = EXCLUDED.search_attributes
		 WHERE workflow_executions.status IN ('COMPLETED', 'FAILED', 'TERMINATED', 'CONTINUED_AS_NEW')`,
		wf.ID, wf.Namespace, wf.WorkflowType, wf.TaskQueue, wf.Status, wf.Input, wf.ParentWorkflowID, wf.ParentSeqID, wf.CronSchedule, searchAttrs,
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

func (s *WorkflowStore) Get(ctx context.Context, namespace string, id uuid.UUID) (*domain.WorkflowExecution, error) {
	var wf domain.WorkflowExecution
	var input, result []byte
	var errMsg sql.NullString
	var closedAt sql.NullTime
	var parentWFID sql.NullString
	var parentSeqID sql.NullInt64
	var cronSchedule sql.NullString
	var continuedAsNewID sql.NullString
	var searchAttrsJSON []byte
	err := s.store.conn(ctx).QueryRowContext(ctx,
		`SELECT id, namespace, workflow_type, task_queue, status, input, result, error_message, created_at, closed_at, parent_workflow_id, parent_seq_id, cron_schedule, continued_as_new_id, search_attributes
		 FROM workflow_executions WHERE id = $1 AND namespace = $2`, id, namespace,
	).Scan(
		&wf.ID, &wf.Namespace, &wf.WorkflowType, &wf.TaskQueue, &wf.Status,
		&input, &result, &errMsg, &wf.CreatedAt, &closedAt,
		&parentWFID, &parentSeqID, &cronSchedule, &continuedAsNewID,
		&searchAttrsJSON,
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
	if parentWFID.Valid {
		parsed, err := uuid.Parse(parentWFID.String)
		if err != nil {
			return nil, fmt.Errorf("parse parent_workflow_id: %w", err)
		}
		wf.ParentWorkflowID = &parsed
	}
	if parentSeqID.Valid {
		wf.ParentSeqID = parentSeqID.Int64
	}
	if cronSchedule.Valid {
		wf.CronSchedule = cronSchedule.String
	}
	if continuedAsNewID.Valid {
		parsed, err := uuid.Parse(continuedAsNewID.String)
		if err != nil {
			return nil, fmt.Errorf("parse continued_as_new_id: %w", err)
		}
		wf.ContinuedAsNewID = &parsed
	}
	if len(searchAttrsJSON) > 0 {
		json.Unmarshal(searchAttrsJSON, &wf.SearchAttributes)
	}
	return &wf, nil
}

func (s *WorkflowStore) List(ctx context.Context, params port.ListWorkflowsParams) ([]domain.WorkflowExecution, error) {
	query := `SELECT id, namespace, workflow_type, task_queue, status, input, result, error_message, created_at, closed_at, parent_workflow_id, parent_seq_id, cron_schedule, continued_as_new_id, search_attributes
		 FROM workflow_executions WHERE namespace = $1`
	args := []any{params.Namespace}
	argIdx := 2

	if params.StatusFilter != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, params.StatusFilter)
		argIdx++
	}
	if params.TypeFilter != "" {
		query += fmt.Sprintf(" AND workflow_type = $%d", argIdx)
		args = append(args, params.TypeFilter)
		argIdx++
	}
	if params.QueueFilter != "" {
		query += fmt.Sprintf(" AND task_queue = $%d", argIdx)
		args = append(args, params.QueueFilter)
		argIdx++
	}
	if params.Cursor != nil {
		query += fmt.Sprintf(" AND (created_at < $%d OR (created_at = $%d AND id < $%d))", argIdx, argIdx, argIdx+1)
		args = append(args, params.Cursor.CreatedAt, params.Cursor.ID)
		argIdx += 2
	}

	query += " ORDER BY created_at DESC, id DESC"
	query += fmt.Sprintf(" LIMIT $%d", argIdx)
	args = append(args, params.PageSize)

	rows, err := s.store.conn(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workflows []domain.WorkflowExecution
	for rows.Next() {
		var wf domain.WorkflowExecution
		var input, result []byte
		var errMsg sql.NullString
		var closedAt sql.NullTime
		var parentWFID sql.NullString
		var parentSeqID sql.NullInt64
		var cronSchedule sql.NullString
		var continuedAsNewID sql.NullString
		var searchAttrsJSON []byte
		if err := rows.Scan(
			&wf.ID, &wf.Namespace, &wf.WorkflowType, &wf.TaskQueue, &wf.Status,
			&input, &result, &errMsg, &wf.CreatedAt, &closedAt,
			&parentWFID, &parentSeqID, &cronSchedule, &continuedAsNewID,
			&searchAttrsJSON,
		); err != nil {
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
		if parentWFID.Valid {
			parsed, err := uuid.Parse(parentWFID.String)
			if err != nil {
				return nil, fmt.Errorf("parse parent_workflow_id: %w", err)
			}
			wf.ParentWorkflowID = &parsed
		}
		if parentSeqID.Valid {
			wf.ParentSeqID = parentSeqID.Int64
		}
		if cronSchedule.Valid {
			wf.CronSchedule = cronSchedule.String
		}
		if continuedAsNewID.Valid {
			parsed, err := uuid.Parse(continuedAsNewID.String)
			if err != nil {
				return nil, fmt.Errorf("parse continued_as_new_id: %w", err)
			}
			wf.ContinuedAsNewID = &parsed
		}
		if len(searchAttrsJSON) > 0 {
			json.Unmarshal(searchAttrsJSON, &wf.SearchAttributes)
		}
		workflows = append(workflows, wf)
	}
	return workflows, rows.Err()
}

func (s *WorkflowStore) SetContinuedAsNewID(ctx context.Context, id uuid.UUID, newID uuid.UUID) error {
	_, err := s.store.conn(ctx).ExecContext(ctx,
		`UPDATE workflow_executions SET continued_as_new_id = $2 WHERE id = $1`,
		id, newID,
	)
	return err
}

func (s *WorkflowStore) UpsertSearchAttributes(ctx context.Context, id uuid.UUID, attrs map[string]string) error {
	attrsJSON, err := json.Marshal(attrs)
	if err != nil {
		return fmt.Errorf("marshal search_attributes: %w", err)
	}
	_, err = s.store.conn(ctx).ExecContext(ctx,
		`UPDATE workflow_executions SET search_attributes = search_attributes || $2, updated_at = NOW() WHERE id = $1`,
		id, attrsJSON,
	)
	return err
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
