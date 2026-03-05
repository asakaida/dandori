package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/asakaida/dandori/internal/domain"
)

type ActivityTaskStore struct {
	store *Store
}

func (s *ActivityTaskStore) Enqueue(ctx context.Context, task domain.ActivityTask) error {
	var retryPolicyJSON []byte
	if task.RetryPolicy != nil {
		var err error
		retryPolicyJSON, err = json.Marshal(task.RetryPolicy)
		if err != nil {
			return fmt.Errorf("marshal retry_policy: %w", err)
		}
	}

	var timeoutInterval *string
	if task.StartToCloseTimeout > 0 {
		s := durationToInterval(task.StartToCloseTimeout)
		timeoutInterval = &s
	}

	_, err := s.store.conn(ctx).ExecContext(ctx,
		`INSERT INTO activity_tasks
			(queue_name, workflow_id, activity_type, activity_input, activity_seq_id,
			 start_to_close_timeout, retry_policy, attempt, max_attempts, status, scheduled_at)
		 VALUES ($1, $2, $3, $4, $5, $6::interval, $7, $8, $9, 'PENDING', COALESCE($10, NOW()))`,
		task.QueueName, task.WorkflowID, task.ActivityType, task.ActivityInput, task.ActivitySeqID,
		timeoutInterval, retryPolicyJSON, task.Attempt, task.MaxAttempts,
		nullTimeIfZero(task.ScheduledAt),
	)
	return err
}

func (s *ActivityTaskStore) Poll(ctx context.Context, queueName string, workerID string) (*domain.ActivityTask, error) {
	var task domain.ActivityTask
	var retryPolicyJSON []byte
	var timeoutMicroseconds sql.NullFloat64
	var timeoutAt sql.NullTime

	err := s.store.conn(ctx).QueryRowContext(ctx,
		`UPDATE activity_tasks SET
			status = 'RUNNING',
			started_at = NOW(),
			locked_by = $2,
			locked_until = NOW() + INTERVAL '30 seconds',
			timeout_at = CASE
				WHEN start_to_close_timeout IS NOT NULL THEN NOW() + start_to_close_timeout
				ELSE NULL
			END
		 WHERE id = (
			SELECT id FROM activity_tasks
			WHERE queue_name = $1 AND status = 'PENDING' AND scheduled_at <= NOW()
			ORDER BY scheduled_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, queue_name, workflow_id, activity_type, activity_input, activity_seq_id,
			EXTRACT(EPOCH FROM start_to_close_timeout) * 1000000, attempt, max_attempts,
			retry_policy, status, scheduled_at, timeout_at`,
		queueName, workerID,
	).Scan(
		&task.ID, &task.QueueName, &task.WorkflowID, &task.ActivityType, &task.ActivityInput,
		&task.ActivitySeqID, &timeoutMicroseconds, &task.Attempt, &task.MaxAttempts,
		&retryPolicyJSON, &task.Status, &task.ScheduledAt, &timeoutAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNoTaskAvailable
	}
	if err != nil {
		return nil, err
	}

	if timeoutMicroseconds.Valid {
		task.StartToCloseTimeout = time.Duration(int64(timeoutMicroseconds.Float64)) * time.Microsecond
	}
	if timeoutAt.Valid {
		task.TimeoutAt = &timeoutAt.Time
	}
	if retryPolicyJSON != nil {
		task.RetryPolicy = &domain.RetryPolicy{}
		if err := json.Unmarshal(retryPolicyJSON, task.RetryPolicy); err != nil {
			return nil, fmt.Errorf("unmarshal retry_policy: %w", err)
		}
	}

	return &task, nil
}

func (s *ActivityTaskStore) Complete(ctx context.Context, taskID int64) error {
	res, err := s.store.conn(ctx).ExecContext(ctx,
		`UPDATE activity_tasks SET status = 'COMPLETED' WHERE id = $1 AND status = 'RUNNING'`,
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

func (s *ActivityTaskStore) GetByID(ctx context.Context, taskID int64) (*domain.ActivityTask, error) {
	var task domain.ActivityTask
	var retryPolicyJSON []byte
	var timeoutMicroseconds sql.NullFloat64
	var timeoutAt sql.NullTime

	err := s.store.conn(ctx).QueryRowContext(ctx,
		`SELECT id, queue_name, workflow_id, activity_type, activity_input, activity_seq_id,
			EXTRACT(EPOCH FROM start_to_close_timeout) * 1000000, attempt, max_attempts,
			retry_policy, status, scheduled_at, timeout_at
		 FROM activity_tasks WHERE id = $1`, taskID,
	).Scan(
		&task.ID, &task.QueueName, &task.WorkflowID, &task.ActivityType, &task.ActivityInput,
		&task.ActivitySeqID, &timeoutMicroseconds, &task.Attempt, &task.MaxAttempts,
		&retryPolicyJSON, &task.Status, &task.ScheduledAt, &timeoutAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}

	if timeoutMicroseconds.Valid {
		task.StartToCloseTimeout = time.Duration(int64(timeoutMicroseconds.Float64)) * time.Microsecond
	}
	if timeoutAt.Valid {
		task.TimeoutAt = &timeoutAt.Time
	}
	if retryPolicyJSON != nil {
		task.RetryPolicy = &domain.RetryPolicy{}
		if err := json.Unmarshal(retryPolicyJSON, task.RetryPolicy); err != nil {
			return nil, fmt.Errorf("unmarshal retry_policy: %w", err)
		}
	}

	return &task, nil
}

func (s *ActivityTaskStore) GetTimedOut(ctx context.Context) ([]domain.ActivityTask, error) {
	rows, err := s.store.conn(ctx).QueryContext(ctx,
		`SELECT id, queue_name, workflow_id, activity_type, activity_input, activity_seq_id,
			EXTRACT(EPOCH FROM start_to_close_timeout) * 1000000, attempt, max_attempts,
			retry_policy, status, scheduled_at, timeout_at
		 FROM activity_tasks
		 WHERE status = 'RUNNING' AND timeout_at < NOW()`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []domain.ActivityTask
	for rows.Next() {
		var task domain.ActivityTask
		var retryPolicyJSON []byte
		var timeoutMicroseconds sql.NullFloat64
		var timeoutAt sql.NullTime

		if err := rows.Scan(
			&task.ID, &task.QueueName, &task.WorkflowID, &task.ActivityType, &task.ActivityInput,
			&task.ActivitySeqID, &timeoutMicroseconds, &task.Attempt, &task.MaxAttempts,
			&retryPolicyJSON, &task.Status, &task.ScheduledAt, &timeoutAt,
		); err != nil {
			return nil, err
		}

		if timeoutMicroseconds.Valid {
			task.StartToCloseTimeout = time.Duration(int64(timeoutMicroseconds.Float64)) * time.Microsecond
		}
		if timeoutAt.Valid {
			task.TimeoutAt = &timeoutAt.Time
		}
		if retryPolicyJSON != nil {
			task.RetryPolicy = &domain.RetryPolicy{}
			if err := json.Unmarshal(retryPolicyJSON, task.RetryPolicy); err != nil {
				return nil, fmt.Errorf("unmarshal retry_policy: %w", err)
			}
		}

		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *ActivityTaskStore) Requeue(ctx context.Context, taskID int64, scheduledAt time.Time) error {
	_, err := s.store.conn(ctx).ExecContext(ctx,
		`UPDATE activity_tasks
		 SET status = 'PENDING', attempt = attempt + 1, timeout_at = NULL,
			 locked_by = NULL, locked_until = NULL, scheduled_at = $2
		 WHERE id = $1`,
		taskID, scheduledAt,
	)
	return err
}

func (s *ActivityTaskStore) RecoverStaleTasks(ctx context.Context) (int, error) {
	res, err := s.store.conn(ctx).ExecContext(ctx,
		`UPDATE activity_tasks SET status = 'PENDING', locked_by = NULL, locked_until = NULL
		 WHERE status = 'RUNNING' AND locked_until < NOW()`,
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

func durationToInterval(d time.Duration) string {
	return fmt.Sprintf("%d microseconds", d.Microseconds())
}
