package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asakaida/dandori/internal/domain"
)

func TestAdvisoryLock_SameWorkflow_Serialized(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	// Enqueue two workflow tasks for the same workflow
	require.NoError(t, store.WorkflowTasks().Enqueue(ctx, domain.WorkflowTask{
		QueueName: "default", WorkflowID: wfID,
	}))
	require.NoError(t, store.WorkflowTasks().Enqueue(ctx, domain.WorkflowTask{
		QueueName: "default", WorkflowID: wfID,
	}))

	task1, err := store.WorkflowTasks().Poll(ctx, "default", "worker-1")
	require.NoError(t, err)
	task2, err := store.WorkflowTasks().Poll(ctx, "default", "worker-2")
	require.NoError(t, err)

	var mu sync.Mutex
	var timestamps []time.Time

	var wg sync.WaitGroup
	wg.Add(2)

	// Both goroutines try GetByID (advisory lock) within a tx concurrently
	for _, taskID := range []int64{task1.ID, task2.ID} {
		go func(tid int64) {
			defer wg.Done()
			err := store.RunInTx(ctx, func(txCtx context.Context) error {
				_, err := store.WorkflowTasks().GetByID(txCtx, tid)
				if err != nil {
					return err
				}
				// Simulate work while holding the lock
				time.Sleep(200 * time.Millisecond)
				mu.Lock()
				timestamps = append(timestamps, time.Now())
				mu.Unlock()
				return store.WorkflowTasks().Complete(txCtx, tid)
			})
			assert.NoError(t, err)
		}(taskID)
	}

	wg.Wait()

	require.Len(t, timestamps, 2)
	diff := timestamps[1].Sub(timestamps[0])
	if diff < 0 {
		diff = -diff
	}
	// If serialized, the gap should be >= 200ms (one waited for the other)
	if diff < 150*time.Millisecond {
		t.Errorf("expected serialized execution (gap >= 150ms), got gap = %v", diff)
	}
}

func TestAdvisoryLock_DifferentWorkflows_Concurrent(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID1 := setupWorkflow(t, ctx, store.Workflows())
	wfID2 := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.WorkflowTasks().Enqueue(ctx, domain.WorkflowTask{
		QueueName: "default", WorkflowID: wfID1,
	}))
	require.NoError(t, store.WorkflowTasks().Enqueue(ctx, domain.WorkflowTask{
		QueueName: "default", WorkflowID: wfID2,
	}))

	task1, err := store.WorkflowTasks().Poll(ctx, "default", "worker-1")
	require.NoError(t, err)
	task2, err := store.WorkflowTasks().Poll(ctx, "default", "worker-2")
	require.NoError(t, err)

	var mu sync.Mutex
	var timestamps []time.Time
	start := time.Now()

	var wg sync.WaitGroup
	wg.Add(2)

	for _, taskID := range []int64{task1.ID, task2.ID} {
		go func(tid int64) {
			defer wg.Done()
			err := store.RunInTx(ctx, func(txCtx context.Context) error {
				_, err := store.WorkflowTasks().GetByID(txCtx, tid)
				if err != nil {
					return err
				}
				time.Sleep(200 * time.Millisecond)
				mu.Lock()
				timestamps = append(timestamps, time.Now())
				mu.Unlock()
				return store.WorkflowTasks().Complete(txCtx, tid)
			})
			assert.NoError(t, err)
		}(taskID)
	}

	wg.Wait()

	elapsed := time.Since(start)
	// Different workflows should run concurrently, total time ~200ms not ~400ms
	if elapsed > 350*time.Millisecond {
		t.Errorf("expected concurrent execution (total < 350ms), got %v", elapsed)
	}

	require.Len(t, timestamps, 2)
}

func TestAdvisoryLock_NoLockOutsideTransaction(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.WorkflowTasks().Enqueue(ctx, domain.WorkflowTask{
		QueueName: "default", WorkflowID: wfID,
	}))
	task, err := store.WorkflowTasks().Poll(ctx, "default", "worker-1")
	require.NoError(t, err)

	// GetByID outside a transaction should succeed without advisory lock
	got, err := store.WorkflowTasks().GetByID(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, task.ID, got.ID)
	assert.Equal(t, wfID, got.WorkflowID)

}
