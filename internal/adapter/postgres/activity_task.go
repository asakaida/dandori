package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/google/uuid"
)

const activityTaskColumns = `id, namespace, queue_name, workflow_id, activity_type, activity_input, activity_seq_id,
	EXTRACT(EPOCH FROM start_to_close_timeout) * 1000000, attempt, max_attempts,
	retry_policy, status, scheduled_at, timeout_at,
	heartbeat_at, EXTRACT(EPOCH FROM heartbeat_timeout) * 1000000,
	EXTRACT(EPOCH FROM schedule_to_close_timeout) * 1000000, schedule_to_close_timeout_at,
	EXTRACT(EPOCH FROM schedule_to_start_timeout) * 1000000, schedule_to_start_timeout_at`

type activityTaskScanVars struct {
	retryPolicyJSON                 []byte
	timeoutMicroseconds             sql.NullFloat64
	timeoutAt                       sql.NullTime
	heartbeatAt                     sql.NullTime
	heartbeatTimeoutMicroseconds    sql.NullFloat64
	schedCloseMicroseconds          sql.NullFloat64
	schedCloseTimeoutAt             sql.NullTime
	schedStartMicroseconds          sql.NullFloat64
	schedStartTimeoutAt             sql.NullTime
}

func (v *activityTaskScanVars) scanArgs(task *domain.ActivityTask) []any {
	return []any{
		&task.ID, &task.Namespace, &task.QueueName, &task.WorkflowID, &task.ActivityType, &task.ActivityInput,
		&task.ActivitySeqID, &v.timeoutMicroseconds, &task.Attempt, &task.MaxAttempts,
		&v.retryPolicyJSON, &task.Status, &task.ScheduledAt, &v.timeoutAt,
		&v.heartbeatAt, &v.heartbeatTimeoutMicroseconds,
		&v.schedCloseMicroseconds, &v.schedCloseTimeoutAt,
		&v.schedStartMicroseconds, &v.schedStartTimeoutAt,
	}
}

func (v *activityTaskScanVars) apply(task *domain.ActivityTask) error {
	if v.timeoutMicroseconds.Valid {
		task.StartToCloseTimeout = time.Duration(int64(v.timeoutMicroseconds.Float64)) * time.Microsecond
	}
	if v.timeoutAt.Valid {
		task.TimeoutAt = &v.timeoutAt.Time
	}
	if v.heartbeatAt.Valid {
		task.HeartbeatAt = &v.heartbeatAt.Time
	}
	if v.heartbeatTimeoutMicroseconds.Valid {
		task.HeartbeatTimeout = time.Duration(int64(v.heartbeatTimeoutMicroseconds.Float64)) * time.Microsecond
	}
	if v.schedCloseMicroseconds.Valid {
		task.ScheduleToCloseTimeout = time.Duration(int64(v.schedCloseMicroseconds.Float64)) * time.Microsecond
	}
	if v.schedCloseTimeoutAt.Valid {
		task.ScheduleToCloseTimeoutAt = &v.schedCloseTimeoutAt.Time
	}
	if v.schedStartMicroseconds.Valid {
		task.ScheduleToStartTimeout = time.Duration(int64(v.schedStartMicroseconds.Float64)) * time.Microsecond
	}
	if v.schedStartTimeoutAt.Valid {
		task.ScheduleToStartTimeoutAt = &v.schedStartTimeoutAt.Time
	}
	if v.retryPolicyJSON != nil {
		task.RetryPolicy = &domain.RetryPolicy{}
		if err := json.Unmarshal(v.retryPolicyJSON, task.RetryPolicy); err != nil {
			return fmt.Errorf("unmarshal retry_policy: %w", err)
		}
	}
	return nil
}

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

	var heartbeatInterval *string
	if task.HeartbeatTimeout > 0 {
		s := durationToInterval(task.HeartbeatTimeout)
		heartbeatInterval = &s
	}

	var schedCloseInterval *string
	if task.ScheduleToCloseTimeout > 0 {
		s := durationToInterval(task.ScheduleToCloseTimeout)
		schedCloseInterval = &s
	}

	var schedStartInterval *string
	if task.ScheduleToStartTimeout > 0 {
		s := durationToInterval(task.ScheduleToStartTimeout)
		schedStartInterval = &s
	}

	_, err := s.store.conn(ctx).ExecContext(ctx,
		`INSERT INTO activity_tasks
			(namespace, queue_name, workflow_id, activity_type, activity_input, activity_seq_id,
			 start_to_close_timeout, heartbeat_timeout, retry_policy, attempt, max_attempts, status, scheduled_at,
			 schedule_to_close_timeout, schedule_to_close_timeout_at,
			 schedule_to_start_timeout, schedule_to_start_timeout_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::interval, $8::interval, $9, $10, $11, 'PENDING', COALESCE($12, NOW()),
			 $13::interval, $14, $15::interval, $16)`,
		task.Namespace, task.QueueName, task.WorkflowID, task.ActivityType, task.ActivityInput, task.ActivitySeqID,
		timeoutInterval, heartbeatInterval, retryPolicyJSON, task.Attempt, task.MaxAttempts,
		nullTimeIfZero(task.ScheduledAt),
		schedCloseInterval, task.ScheduleToCloseTimeoutAt,
		schedStartInterval, task.ScheduleToStartTimeoutAt,
	)
	return err
}

func (s *ActivityTaskStore) Poll(ctx context.Context, namespace string, queueName string, workerID string) (*domain.ActivityTask, error) {
	var task domain.ActivityTask
	var v activityTaskScanVars

	err := s.store.conn(ctx).QueryRowContext(ctx,
		`UPDATE activity_tasks SET
			status = 'RUNNING',
			started_at = NOW(),
			locked_by = $3,
			locked_until = NOW() + INTERVAL '30 seconds',
			timeout_at = CASE
				WHEN start_to_close_timeout IS NOT NULL THEN NOW() + start_to_close_timeout
				ELSE NULL
			END,
			heartbeat_at = CASE
				WHEN heartbeat_timeout IS NOT NULL THEN NOW()
				ELSE NULL
			END,
			schedule_to_start_timeout_at = NULL
		 WHERE id = (
			SELECT id FROM activity_tasks
			WHERE namespace = $1 AND queue_name = $2 AND status = 'PENDING' AND scheduled_at <= NOW()
			ORDER BY scheduled_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		 )
		 RETURNING `+activityTaskColumns,
		namespace, queueName, workerID,
	).Scan(v.scanArgs(&task)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNoTaskAvailable
	}
	if err != nil {
		return nil, err
	}

	if err := v.apply(&task); err != nil {
		return nil, err
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

func (s *ActivityTaskStore) CompletePending(ctx context.Context, taskID int64) error {
	res, err := s.store.conn(ctx).ExecContext(ctx,
		`UPDATE activity_tasks SET status = 'COMPLETED' WHERE id = $1 AND status = 'PENDING'`,
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
	var v activityTaskScanVars

	err := s.store.conn(ctx).QueryRowContext(ctx,
		`SELECT `+activityTaskColumns+` FROM activity_tasks WHERE id = $1`, taskID,
	).Scan(v.scanArgs(&task)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}

	if err := v.apply(&task); err != nil {
		return nil, err
	}

	return &task, nil
}

func (s *ActivityTaskStore) GetTimedOut(ctx context.Context) ([]domain.ActivityTask, error) {
	return s.queryActivityTasks(ctx,
		`SELECT `+activityTaskColumns+`
		 FROM activity_tasks
		 WHERE status = 'RUNNING' AND timeout_at < NOW()`)
}

func (s *ActivityTaskStore) GetHeartbeatTimedOut(ctx context.Context) ([]domain.ActivityTask, error) {
	return s.queryActivityTasks(ctx,
		`SELECT `+activityTaskColumns+`
		 FROM activity_tasks
		 WHERE status = 'RUNNING' AND heartbeat_timeout IS NOT NULL AND heartbeat_at + heartbeat_timeout < NOW()`)
}

func (s *ActivityTaskStore) GetScheduleToCloseTimedOut(ctx context.Context) ([]domain.ActivityTask, error) {
	return s.queryActivityTasks(ctx,
		`SELECT `+activityTaskColumns+`
		 FROM activity_tasks
		 WHERE status IN ('PENDING', 'RUNNING') AND schedule_to_close_timeout_at IS NOT NULL AND schedule_to_close_timeout_at < NOW()`)
}

func (s *ActivityTaskStore) GetScheduleToStartTimedOut(ctx context.Context) ([]domain.ActivityTask, error) {
	return s.queryActivityTasks(ctx,
		`SELECT `+activityTaskColumns+`
		 FROM activity_tasks
		 WHERE status = 'PENDING' AND schedule_to_start_timeout_at IS NOT NULL AND schedule_to_start_timeout_at < NOW()`)
}

func (s *ActivityTaskStore) queryActivityTasks(ctx context.Context, query string) ([]domain.ActivityTask, error) {
	rows, err := s.store.conn(ctx).QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []domain.ActivityTask
	for rows.Next() {
		var task domain.ActivityTask
		var v activityTaskScanVars

		if err := rows.Scan(v.scanArgs(&task)...); err != nil {
			return nil, err
		}

		if err := v.apply(&task); err != nil {
			return nil, err
		}

		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *ActivityTaskStore) UpdateHeartbeat(ctx context.Context, taskID int64) error {
	res, err := s.store.conn(ctx).ExecContext(ctx,
		`UPDATE activity_tasks SET heartbeat_at = NOW() WHERE id = $1 AND status = 'RUNNING'`,
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
		return domain.ErrTaskNotFound
	}
	return nil
}

func (s *ActivityTaskStore) Requeue(ctx context.Context, taskID int64, scheduledAt time.Time) error {
	_, err := s.store.conn(ctx).ExecContext(ctx,
		`UPDATE activity_tasks
		 SET status = 'PENDING', attempt = attempt + 1, timeout_at = NULL,
			 locked_by = NULL, locked_until = NULL, scheduled_at = $2::timestamptz,
			 schedule_to_start_timeout_at = CASE
				 WHEN schedule_to_start_timeout IS NOT NULL THEN $2::timestamptz + schedule_to_start_timeout
				 ELSE NULL
			 END
		 WHERE id = $1`,
		taskID, scheduledAt,
	)
	return err
}

func (s *ActivityTaskStore) DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error {
	_, err := s.store.conn(ctx).ExecContext(ctx,
		`DELETE FROM activity_tasks WHERE workflow_id = $1`, workflowID,
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
