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

	err = store.Timers().MarkFired(ctx, fired[0].ID)
	require.NoError(t, err)

	// After marking, no more fired timers
	fired, err = store.Timers().GetFired(ctx)
	require.NoError(t, err)
	assert.Len(t, fired, 0)
}

func TestTimerStore_GetFired_FutureTimerNotReturned(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	err := store.Timers().Create(ctx, domain.Timer{
		WorkflowID: wfID,
		SeqID:      1,
		FireAt:     time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)

	fired, err := store.Timers().GetFired(ctx)
	require.NoError(t, err)
	assert.Len(t, fired, 0)
}
