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

	task, err := store.ActivityTasks().Poll(ctx, "default", "worker-1")
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

	_, err := store.ActivityTasks().Poll(ctx, "default", "worker-1")
	assert.ErrorIs(t, err, domain.ErrNoTaskAvailable)
}

func TestActivityTaskStore_Poll_SetsTimeoutAt(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	timeout := 60 * time.Second
	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
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
	task, err := store.ActivityTasks().Poll(ctx, "default", "worker-1")
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
		QueueName:     "default",
		WorkflowID:    wfID,
		ActivityType:  "process",
		ActivityInput: json.RawMessage(`{}`),
		ActivitySeqID: 0,
		Attempt:       1,
		MaxAttempts:   1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "worker-1")
	require.NoError(t, err)
	assert.Nil(t, task.TimeoutAt)
}

func TestActivityTaskStore_Poll_SkipLocked(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		QueueName:     "default",
		WorkflowID:    wfID,
		ActivityType:  "process",
		ActivityInput: json.RawMessage(`{}`),
		ActivitySeqID: 0,
		Attempt:       1,
		MaxAttempts:   1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "worker-1")
	require.NoError(t, err)
	require.NotNil(t, task)

	_, err = store.ActivityTasks().Poll(ctx, "default", "worker-2")
	assert.ErrorIs(t, err, domain.ErrNoTaskAvailable)
}

func TestActivityTaskStore_GetTimedOut(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		QueueName:           "default",
		WorkflowID:          wfID,
		ActivityType:        "slow-task",
		ActivityInput:       json.RawMessage(`{}`),
		ActivitySeqID:       0,
		StartToCloseTimeout: 1 * time.Second,
		Attempt:             1,
		MaxAttempts:         1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "worker-1")
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
		QueueName:           "default",
		WorkflowID:          wfID,
		ActivityType:        "retry-task",
		ActivityInput:       json.RawMessage(`{}`),
		ActivitySeqID:       0,
		StartToCloseTimeout: 30 * time.Second,
		Attempt:             1,
		MaxAttempts:         3,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "worker-1")
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
		QueueName:     "default",
		WorkflowID:    wfID,
		ActivityType:  "task",
		ActivityInput: json.RawMessage(`{}`),
		ActivitySeqID: 0,
		Attempt:       1,
		MaxAttempts:   1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "worker-1")
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

func TestActivityTaskStore_RecoverStaleTasks(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.ActivityTasks().Enqueue(ctx, domain.ActivityTask{
		QueueName:     "default",
		WorkflowID:    wfID,
		ActivityType:  "stale-task",
		ActivityInput: json.RawMessage(`{}`),
		ActivitySeqID: 0,
		Attempt:       1,
		MaxAttempts:   1,
	}))

	task, err := store.ActivityTasks().Poll(ctx, "default", "worker-1")
	require.NoError(t, err)

	_, err = testDB.ExecContext(ctx,
		`UPDATE activity_tasks SET locked_until = NOW() - INTERVAL '1 minute' WHERE id = $1`, task.ID)
	require.NoError(t, err)

	n, err := store.ActivityTasks().RecoverStaleTasks(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	recovered, err := store.ActivityTasks().Poll(ctx, "default", "worker-2")
	require.NoError(t, err)
	assert.Equal(t, task.ID, recovered.ID)
}
