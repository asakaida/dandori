package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asakaida/dandori/internal/domain"
)

func TestWorkflowTaskStore_Enqueue_Poll_Complete(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	err := store.WorkflowTasks().Enqueue(ctx, domain.WorkflowTask{
		QueueName:  "default",
		WorkflowID: wfID,
	})
	require.NoError(t, err)

	task, err := store.WorkflowTasks().Poll(ctx, "default", "worker-1")
	require.NoError(t, err)
	assert.Equal(t, wfID, task.WorkflowID)
	assert.Equal(t, domain.TaskStatusRunning, task.Status)

	err = store.WorkflowTasks().Complete(ctx, task.ID)
	require.NoError(t, err)
}

func TestWorkflowTaskStore_Poll_NoTask(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	_, err := store.WorkflowTasks().Poll(ctx, "default", "worker-1")
	assert.ErrorIs(t, err, domain.ErrNoTaskAvailable)
}

func TestWorkflowTaskStore_Complete_AlreadyCompleted(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.WorkflowTasks().Enqueue(ctx, domain.WorkflowTask{
		QueueName: "default", WorkflowID: wfID,
	}))
	task, err := store.WorkflowTasks().Poll(ctx, "default", "worker-1")
	require.NoError(t, err)
	require.NoError(t, store.WorkflowTasks().Complete(ctx, task.ID))

	err = store.WorkflowTasks().Complete(ctx, task.ID)
	assert.ErrorIs(t, err, domain.ErrTaskAlreadyCompleted)
}

func TestWorkflowTaskStore_GetByID(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.WorkflowTasks().Enqueue(ctx, domain.WorkflowTask{
		QueueName: "default", WorkflowID: wfID,
	}))
	polled, err := store.WorkflowTasks().Poll(ctx, "default", "worker-1")
	require.NoError(t, err)

	// GetByID with advisory lock requires a transaction
	err = store.RunInTx(ctx, func(ctx context.Context) error {
		task, err := store.WorkflowTasks().GetByID(ctx, polled.ID)
		if err != nil {
			return err
		}
		assert.Equal(t, polled.ID, task.ID)
		assert.Equal(t, wfID, task.WorkflowID)
		return nil
	})
	require.NoError(t, err)
}

func TestWorkflowTaskStore_GetByID_NotFound(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	_, err := store.WorkflowTasks().GetByID(ctx, 99999)
	assert.ErrorIs(t, err, domain.ErrTaskNotFound)
}

func TestWorkflowTaskStore_Poll_SkipLocked(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	// Enqueue a single task
	require.NoError(t, store.WorkflowTasks().Enqueue(ctx, domain.WorkflowTask{
		QueueName: "default", WorkflowID: wfID,
	}))

	// Poll from worker-1
	task, err := store.WorkflowTasks().Poll(ctx, "default", "worker-1")
	require.NoError(t, err)
	require.NotNil(t, task)

	// Poll from worker-2 should get no task (SKIP LOCKED)
	_, err = store.WorkflowTasks().Poll(ctx, "default", "worker-2")
	assert.ErrorIs(t, err, domain.ErrNoTaskAvailable)
}

func TestWorkflowTaskStore_RecoverStaleTasks(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.WorkflowTasks().Enqueue(ctx, domain.WorkflowTask{
		QueueName: "default", WorkflowID: wfID,
	}))

	// Poll to make it RUNNING
	task, err := store.WorkflowTasks().Poll(ctx, "default", "worker-1")
	require.NoError(t, err)

	// Manually set locked_until to past
	_, err = testDB.ExecContext(ctx,
		`UPDATE workflow_tasks SET locked_until = NOW() - INTERVAL '1 minute' WHERE id = $1`, task.ID)
	require.NoError(t, err)

	n, err := store.WorkflowTasks().RecoverStaleTasks(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	// Should be pollable again
	recovered, err := store.WorkflowTasks().Poll(ctx, "default", "worker-2")
	require.NoError(t, err)
	assert.Equal(t, task.ID, recovered.ID)
}

func TestWorkflowTaskStore_Poll_RespectsScheduledAt(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	// Enqueue a task scheduled in the future
	require.NoError(t, store.WorkflowTasks().Enqueue(ctx, domain.WorkflowTask{
		QueueName:   "default",
		WorkflowID:  wfID,
		ScheduledAt: time.Now().Add(1 * time.Hour),
	}))

	// Should not be pollable yet
	_, err := store.WorkflowTasks().Poll(ctx, "default", "worker-1")
	assert.ErrorIs(t, err, domain.ErrNoTaskAvailable)
}
