package postgres_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asakaida/dandori/internal/domain"
)

func TestActivityTaskStore_Enqueue_Poll_Complete(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	err := store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:           "default",
		WorkflowID:          wfID,
		ActivityType:        "send-email",
		ActivityInput:       json.RawMessage(`{"to":"test@example.com"}`),
		ActivitySeqID:       0,
		StartToCloseTimeout: 30 * time.Second,
		Attempt:             1,
		MaxAttempts:         3,
		RetryPolicy: &domain.RetryPolicy{
			MaxAttempts:        3,
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaxInterval:        30 * time.Second,
		},
	})
	require.NoError(t, err)

	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)
	assert.Equal(t, wfID, task.WorkflowID)
	assert.Equal(t, "send-email", task.ActivityType)
	assert.Equal(t, domain.TaskStatusRunning, task.Status)
	assert.Equal(t, int64(0), task.ActivitySeqID)
	assert.Equal(t, 1, task.Attempt)
	assert.Equal(t, 3, task.MaxAttempts)
	assert.NotNil(t, task.RetryPolicy)
	assert.Equal(t, 3, task.RetryPolicy.MaxAttempts)

	err = store.ActivityTasks().Complete(ctx, task.ID)
	require.NoError(t, err)
}

func TestActivityTaskStore_Poll_NoTask(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	_, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	assert.ErrorIs(t, err, domain.ErrNoTaskAvailable)
}

func TestActivityTaskStore_Poll_SetsTimeoutAt(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	timeout := 60 * time.Second
	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:           "default",
		WorkflowID:          wfID,
		ActivityType:        "process",
		ActivityInput:       json.RawMessage(`{}`),
		ActivitySeqID:       0,
		StartToCloseTimeout: timeout,
		Attempt:             1,
		MaxAttempts:         1,
	}))

	before := time.Now()
	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)

	require.NotNil(t, task.TimeoutAt)
	// timeout_at should be approximately NOW() + 60s
	assert.WithinDuration(t, before.Add(timeout), *task.TimeoutAt, 5*time.Second)
}

func TestActivityTaskStore_Poll_NoTimeoutWhenNotSet(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:     "default",
		WorkflowID:    wfID,
		ActivityType:  "process",
		ActivityInput: json.RawMessage(`{}`),
		ActivitySeqID: 0,
		Attempt:       1,
		MaxAttempts:   1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)
	assert.Nil(t, task.TimeoutAt)
}

func TestActivityTaskStore_Poll_SkipLocked(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:     "default",
		WorkflowID:    wfID,
		ActivityType:  "process",
		ActivityInput: json.RawMessage(`{}`),
		ActivitySeqID: 0,
		Attempt:       1,
		MaxAttempts:   1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)
	require.NotNil(t, task)

	_, err = store.ActivityTasks().Poll(ctx, "default", "default", "worker-2")
	assert.ErrorIs(t, err, domain.ErrNoTaskAvailable)
}

func TestActivityTaskStore_GetTimedOut(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:           "default",
		WorkflowID:          wfID,
		ActivityType:        "slow-task",
		ActivityInput:       json.RawMessage(`{}`),
		ActivitySeqID:       0,
		StartToCloseTimeout: 1 * time.Second,
		Attempt:             1,
		MaxAttempts:         1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)

	// Set timeout_at to past
	_, err = testDB.ExecContext(ctx,
		`UPDATE activity_tasks SET timeout_at = NOW() - INTERVAL '1 second' WHERE id = $1`, task.ID)
	require.NoError(t, err)

	timedOut, err := store.ActivityTasks().GetTimedOut(ctx)
	require.NoError(t, err)
	require.Len(t, timedOut, 1)
	assert.Equal(t, task.ID, timedOut[0].ID)
}

func TestActivityTaskStore_Requeue(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:           "default",
		WorkflowID:          wfID,
		ActivityType:        "retry-task",
		ActivityInput:       json.RawMessage(`{}`),
		ActivitySeqID:       0,
		StartToCloseTimeout: 30 * time.Second,
		Attempt:             1,
		MaxAttempts:         3,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)
	assert.Equal(t, 1, task.Attempt)

	nextSchedule := time.Now().Add(5 * time.Second)
	err = store.ActivityTasks().Requeue(ctx, task.ID, nextSchedule)
	require.NoError(t, err)

	requeued, err := store.ActivityTasks().GetByID(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusPending, requeued.Status)
	assert.Equal(t, 2, requeued.Attempt)
	assert.Nil(t, requeued.TimeoutAt)
}

func TestActivityTaskStore_Complete_AlreadyCompleted(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:     "default",
		WorkflowID:    wfID,
		ActivityType:  "task",
		ActivityInput: json.RawMessage(`{}`),
		ActivitySeqID: 0,
		Attempt:       1,
		MaxAttempts:   1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)
	require.NoError(t, store.ActivityTasks().Complete(ctx, task.ID))

	err = store.ActivityTasks().Complete(ctx, task.ID)
	assert.ErrorIs(t, err, domain.ErrTaskAlreadyCompleted)
}

func TestActivityTaskStore_GetByID_NotFound(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	_, err := store.ActivityTasks().GetByID(ctx, 99999)
	assert.ErrorIs(t, err, domain.ErrTaskNotFound)
}

func TestActivityTaskStore_DeleteByWorkflowID(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:     "default",
		WorkflowID:    wfID,
		ActivityType:  "task-1",
		ActivityInput: json.RawMessage(`{}`),
		ActivitySeqID: 0,
		Attempt:       1,
		MaxAttempts:   1,
	}))

	err := store.ActivityTasks().DeleteByWorkflowID(ctx, wfID)
	require.NoError(t, err)

	_, err = store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	assert.ErrorIs(t, err, domain.ErrNoTaskAvailable)
}

func TestActivityTaskStore_RecoverStaleTasks(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:     "default",
		WorkflowID:    wfID,
		ActivityType:  "stale-task",
		ActivityInput: json.RawMessage(`{}`),
		ActivitySeqID: 0,
		Attempt:       1,
		MaxAttempts:   1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)

	_, err = testDB.ExecContext(ctx,
		`UPDATE activity_tasks SET locked_until = NOW() - INTERVAL '1 minute' WHERE id = $1`, task.ID)
	require.NoError(t, err)

	n, err := store.ActivityTasks().RecoverStaleTasks(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	recovered, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-2")
	require.NoError(t, err)
	assert.Equal(t, task.ID, recovered.ID)
}

func TestActivityTaskStore_Enqueue_WithScheduleToCloseTimeout(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	schedClose := 60 * time.Second
	schedCloseAt := time.Now().Add(schedClose)
	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:                "default",
		WorkflowID:               wfID,
		ActivityType:             "sched-close-task",
		ActivityInput:            json.RawMessage(`{}`),
		ActivitySeqID:            0,
		StartToCloseTimeout:      30 * time.Second,
		ScheduleToCloseTimeout:   schedClose,
		ScheduleToCloseTimeoutAt: &schedCloseAt,
		Attempt:                  1,
		MaxAttempts:              1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)
	assert.Equal(t, schedClose, task.ScheduleToCloseTimeout)
	require.NotNil(t, task.ScheduleToCloseTimeoutAt)
	assert.WithinDuration(t, schedCloseAt, *task.ScheduleToCloseTimeoutAt, 2*time.Second)
}

func TestActivityTaskStore_Enqueue_WithScheduleToStartTimeout(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	schedStart := 10 * time.Second
	schedStartAt := time.Now().Add(schedStart)
	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:                "default",
		WorkflowID:               wfID,
		ActivityType:             "sched-start-task",
		ActivityInput:            json.RawMessage(`{}`),
		ActivitySeqID:            0,
		ScheduleToStartTimeout:   schedStart,
		ScheduleToStartTimeoutAt: &schedStartAt,
		Attempt:                  1,
		MaxAttempts:              1,
	}))

	got, err := store.ActivityTasks().GetByID(ctx, 1)
	// GetByID with unknown ID; use Poll instead to get the actual task
	_ = got
	_ = err

	polled, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)
	assert.Equal(t, schedStart, polled.ScheduleToStartTimeout)
	// After poll, schedule_to_start_timeout_at should be cleared
	assert.Nil(t, polled.ScheduleToStartTimeoutAt)
}

func TestActivityTaskStore_Poll_ClearsScheduleToStartTimeoutAt(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	schedStartAt := time.Now().Add(30 * time.Second)
	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:                "default",
		WorkflowID:               wfID,
		ActivityType:             "sched-start-clear",
		ActivityInput:            json.RawMessage(`{}`),
		ActivitySeqID:            0,
		ScheduleToStartTimeout:   30 * time.Second,
		ScheduleToStartTimeoutAt: &schedStartAt,
		Attempt:                  1,
		MaxAttempts:              1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)
	assert.Nil(t, task.ScheduleToStartTimeoutAt, "schedule_to_start_timeout_at should be NULL after poll")
	assert.Equal(t, 30*time.Second, task.ScheduleToStartTimeout, "schedule_to_start_timeout duration should be preserved")
}

func TestActivityTaskStore_Requeue_MaintainsScheduleToCloseTimeoutAt(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	schedCloseAt := time.Now().Add(60 * time.Second)
	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:                "default",
		WorkflowID:               wfID,
		ActivityType:             "requeue-close",
		ActivityInput:            json.RawMessage(`{}`),
		ActivitySeqID:            0,
		StartToCloseTimeout:      30 * time.Second,
		ScheduleToCloseTimeout:   60 * time.Second,
		ScheduleToCloseTimeoutAt: &schedCloseAt,
		Attempt:                  1,
		MaxAttempts:              3,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)

	nextSchedule := time.Now().Add(5 * time.Second)
	err = store.ActivityTasks().Requeue(ctx, task.ID, nextSchedule)
	require.NoError(t, err)

	requeued, err := store.ActivityTasks().GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.NotNil(t, requeued.ScheduleToCloseTimeoutAt, "schedule_to_close_timeout_at should be maintained after requeue")
	assert.WithinDuration(t, schedCloseAt, *requeued.ScheduleToCloseTimeoutAt, 2*time.Second)
}

func TestActivityTaskStore_Requeue_RecalculatesScheduleToStartTimeoutAt(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	schedStart := 10 * time.Second
	schedStartAt := time.Now().Add(schedStart)
	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:                "default",
		WorkflowID:               wfID,
		ActivityType:             "requeue-start",
		ActivityInput:            json.RawMessage(`{}`),
		ActivitySeqID:            0,
		StartToCloseTimeout:      30 * time.Second,
		ScheduleToStartTimeout:   schedStart,
		ScheduleToStartTimeoutAt: &schedStartAt,
		Attempt:                  1,
		MaxAttempts:              3,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)

	nextSchedule := time.Now().Add(5 * time.Second)
	err = store.ActivityTasks().Requeue(ctx, task.ID, nextSchedule)
	require.NoError(t, err)

	requeued, err := store.ActivityTasks().GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.NotNil(t, requeued.ScheduleToStartTimeoutAt, "schedule_to_start_timeout_at should be recalculated")
	expected := nextSchedule.Add(schedStart)
	assert.WithinDuration(t, expected, *requeued.ScheduleToStartTimeoutAt, 2*time.Second)
}

func TestActivityTaskStore_GetScheduleToCloseTimedOut(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	// Create a PENDING task with past schedule_to_close_timeout_at
	pastTime := time.Now().Add(-1 * time.Second)
	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:                "default",
		WorkflowID:               wfID,
		ActivityType:             "sched-close-pending",
		ActivityInput:            json.RawMessage(`{}`),
		ActivitySeqID:            0,
		ScheduleToCloseTimeout:   1 * time.Second,
		ScheduleToCloseTimeoutAt: &pastTime,
		Attempt:                  1,
		MaxAttempts:              1,
	}))

	timedOut, err := store.ActivityTasks().GetScheduleToCloseTimedOut(ctx)
	require.NoError(t, err)
	require.Len(t, timedOut, 1)
	assert.Equal(t, domain.TaskStatusPending, timedOut[0].Status)
}

func TestActivityTaskStore_GetScheduleToStartTimedOut(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	// Create a PENDING task with past schedule_to_start_timeout_at (scheduled in the future so Poll won't pick it)
	pastTime := time.Now().Add(-1 * time.Second)
	futureSchedule := time.Now().Add(1 * time.Hour)
	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:                "default",
		WorkflowID:               wfID,
		ActivityType:             "sched-start-pending",
		ActivityInput:            json.RawMessage(`{}`),
		ActivitySeqID:            0,
		ScheduleToStartTimeout:   1 * time.Second,
		ScheduleToStartTimeoutAt: &pastTime,
		ScheduledAt:              futureSchedule,
		Attempt:                  1,
		MaxAttempts:              1,
	}))

	// Create a RUNNING task with past schedule_to_start_timeout_at - should NOT appear
	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:                "default",
		WorkflowID:               wfID,
		ActivityType:             "sched-start-running",
		ActivityInput:            json.RawMessage(`{}`),
		ActivitySeqID:            1,
		ScheduleToStartTimeout:   1 * time.Second,
		ScheduleToStartTimeoutAt: &pastTime,
		Attempt:                  1,
		MaxAttempts:              1,
	}))
	// Poll the second task to make it RUNNING (first task is scheduled in the future)
	polled, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)
	assert.Equal(t, "sched-start-running", polled.ActivityType)

	timedOut, err := store.ActivityTasks().GetScheduleToStartTimedOut(ctx)
	require.NoError(t, err)
	// Only the PENDING one should be returned
	require.Len(t, timedOut, 1)
	assert.Equal(t, "sched-start-pending", timedOut[0].ActivityType)
}

func TestActivityTaskStore_CompletePending(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:     "default",
		WorkflowID:    wfID,
		ActivityType:  "pending-complete",
		ActivityInput: json.RawMessage(`{}`),
		ActivitySeqID: 0,
		Attempt:       1,
		MaxAttempts:   1,
	}))

	// Find task ID via direct SQL since we can't Poll without changing status
	var taskID int64
	err := testDB.QueryRowContext(ctx, `SELECT id FROM activity_tasks WHERE activity_type = 'pending-complete'`).Scan(&taskID)
	require.NoError(t, err)

	err = store.ActivityTasks().CompletePending(ctx, taskID)
	require.NoError(t, err)

	got, err := store.ActivityTasks().GetByID(ctx, taskID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusCompleted, got.Status)
}

func TestActivityTaskStore_CompletePending_NotPending(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:     "default",
		WorkflowID:    wfID,
		ActivityType:  "pending-not",
		ActivityInput: json.RawMessage(`{}`),
		ActivitySeqID: 0,
		Attempt:       1,
		MaxAttempts:   1,
	}))

	// Poll to make it RUNNING
	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)

	// CompletePending should fail since task is RUNNING
	err = store.ActivityTasks().CompletePending(ctx, task.ID)
	assert.ErrorIs(t, err, domain.ErrTaskAlreadyCompleted)
}

func TestActivityTaskStore_Enqueue_WithHeartbeat(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:        "default",
		WorkflowID:       wfID,
		ActivityType:     "heartbeat-task",
		ActivityInput:    json.RawMessage(`{}`),
		ActivitySeqID:    0,
		HeartbeatTimeout: 5 * time.Second,
		Attempt:          1,
		MaxAttempts:      1,
	}))

	// Poll to get the actual task ID, then check via GetByID
	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, task.HeartbeatTimeout)
	// After poll, heartbeat_at is initialized since heartbeat_timeout is set
	require.NotNil(t, task.HeartbeatAt)
}

func TestActivityTaskStore_Poll_InitializesHeartbeat(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:        "default",
		WorkflowID:       wfID,
		ActivityType:     "heartbeat-task",
		ActivityInput:    json.RawMessage(`{}`),
		ActivitySeqID:    0,
		HeartbeatTimeout: 10 * time.Second,
		Attempt:          1,
		MaxAttempts:      1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, task.HeartbeatTimeout)
	require.NotNil(t, task.HeartbeatAt, "heartbeat_at should be initialized on poll")
	assert.WithinDuration(t, time.Now(), *task.HeartbeatAt, 5*time.Second)
}

func TestActivityTaskStore_UpdateHeartbeat(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:        "default",
		WorkflowID:       wfID,
		ActivityType:     "heartbeat-task",
		ActivityInput:    json.RawMessage(`{}`),
		ActivitySeqID:    0,
		HeartbeatTimeout: 10 * time.Second,
		Attempt:          1,
		MaxAttempts:      1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)

	err = store.ActivityTasks().UpdateHeartbeat(ctx, task.ID)
	require.NoError(t, err)

	updated, err := store.ActivityTasks().GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.HeartbeatAt)
	assert.WithinDuration(t, time.Now(), *updated.HeartbeatAt, 5*time.Second)
}

func TestActivityTaskStore_UpdateHeartbeat_NotFound(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	err := store.ActivityTasks().UpdateHeartbeat(ctx, 99999)
	assert.ErrorIs(t, err, domain.ErrTaskNotFound)
}

func TestActivityTaskStore_GetHeartbeatTimedOut(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		Namespace:  "default",
		QueueName:        "default",
		WorkflowID:       wfID,
		ActivityType:     "hb-timeout-task",
		ActivityInput:    json.RawMessage(`{}`),
		ActivitySeqID:    0,
		HeartbeatTimeout: 1 * time.Second,
		Attempt:          1,
		MaxAttempts:      1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "default", "worker-1")
	require.NoError(t, err)

	// Set heartbeat_at to past to simulate timeout
	_, err = testDB.ExecContext(ctx,
		`UPDATE activity_tasks SET heartbeat_at = NOW() - INTERVAL '10 seconds' WHERE id = $1`, task.ID)
	require.NoError(t, err)

	timedOut, err := store.ActivityTasks().GetHeartbeatTimedOut(ctx)
	require.NoError(t, err)
	require.Len(t, timedOut, 1)
	assert.Equal(t, task.ID, timedOut[0].ID)
}
