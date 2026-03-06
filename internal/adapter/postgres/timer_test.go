package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asakaida/dandori/internal/domain"
)

func TestTimerStore_Create_GetFired_MarkFired(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	// Create a timer that already fired (fire_at in the past)
	err := store.Timers().Create(ctx, domain.Timer{
		Namespace:  "default",
		WorkflowID: wfID,
		SeqID:      1,
		FireAt:     time.Now().Add(-1 * time.Second),
	})
	require.NoError(t, err)

	fired, err := store.Timers().GetFired(ctx)
	require.NoError(t, err)
	require.Len(t, fired, 1)
	assert.Equal(t, wfID, fired[0].WorkflowID)
	assert.Equal(t, int64(1), fired[0].SeqID)

	ok, err := store.Timers().MarkFired(ctx, fired[0].ID)
	require.NoError(t, err)
	assert.True(t, ok)

	// After marking, no more fired timers
	fired, err = store.Timers().GetFired(ctx)
	require.NoError(t, err)
	assert.Len(t, fired, 0)
}

func TestTimerStore_DeleteByWorkflowID(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.Timers().Create(ctx, domain.Timer{
		Namespace:  "default",
		WorkflowID: wfID,
		SeqID:      1,
		FireAt:     time.Now().Add(-1 * time.Second),
	}))

	err := store.Timers().DeleteByWorkflowID(ctx, wfID)
	require.NoError(t, err)

	fired, err := store.Timers().GetFired(ctx)
	require.NoError(t, err)
	assert.Len(t, fired, 0)
}

func TestTimerStore_GetFired_FutureTimerNotReturned(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	err := store.Timers().Create(ctx, domain.Timer{
		Namespace:  "default",
		WorkflowID: wfID,
		SeqID:      1,
		FireAt:     time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)

	fired, err := store.Timers().GetFired(ctx)
	require.NoError(t, err)
	assert.Len(t, fired, 0)
}

func TestTimerStore_Cancel_Pending(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.Timers().Create(ctx, domain.Timer{
		Namespace:  "default",
		WorkflowID: wfID,
		SeqID:      1,
		FireAt:     time.Now().Add(1 * time.Hour),
	}))

	ok, err := store.Timers().Cancel(ctx, wfID, 1)
	require.NoError(t, err)
	assert.True(t, ok)

	// Canceled timer should not appear in GetFired
	fired, err := store.Timers().GetFired(ctx)
	require.NoError(t, err)
	assert.Len(t, fired, 0)
}

func TestTimerStore_Cancel_AlreadyFired(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.Timers().Create(ctx, domain.Timer{
		Namespace:  "default",
		WorkflowID: wfID,
		SeqID:      1,
		FireAt:     time.Now().Add(-1 * time.Second),
	}))

	fired, err := store.Timers().GetFired(ctx)
	require.NoError(t, err)
	require.Len(t, fired, 1)

	ok, err := store.Timers().MarkFired(ctx, fired[0].ID)
	require.NoError(t, err)
	assert.True(t, ok)

	// Cancel after FIRED should return false
	ok, err = store.Timers().Cancel(ctx, wfID, 1)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestTimerStore_Cancel_NonExistent(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	ok, err := store.Timers().Cancel(ctx, wfID, 999)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestTimerStore_MarkFired_PendingGuard(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.Timers().Create(ctx, domain.Timer{
		Namespace:  "default",
		WorkflowID: wfID,
		SeqID:      1,
		FireAt:     time.Now().Add(-1 * time.Second),
	}))

	fired, err := store.Timers().GetFired(ctx)
	require.NoError(t, err)
	require.Len(t, fired, 1)

	// First MarkFired succeeds
	ok, err := store.Timers().MarkFired(ctx, fired[0].ID)
	require.NoError(t, err)
	assert.True(t, ok)

	// Second MarkFired returns false (already FIRED)
	ok, err = store.Timers().MarkFired(ctx, fired[0].ID)
	require.NoError(t, err)
	assert.False(t, ok)
}
