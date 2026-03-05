package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asakaida/dandori/internal/domain"
)

func TestTxManager_RunInTx_Commit(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	wfID := uuid.New()
	err := store.RunInTx(ctx, func(ctx context.Context) error {
		return store.Workflows().Create(ctx, domain.WorkflowExecution{
			ID:           wfID,
			WorkflowType: "test-wf",
			TaskQueue:    "default",
			Status:       domain.WorkflowStatusRunning,
			Input:        json.RawMessage(`{"key":"value"}`),
		})
	})
	require.NoError(t, err)

	wf, err := store.Workflows().Get(ctx, wfID)
	require.NoError(t, err)
	assert.Equal(t, wfID, wf.ID)
}

func TestTxManager_RunInTx_Rollback(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	wfID := uuid.New()
	err := store.RunInTx(ctx, func(ctx context.Context) error {
		if err := store.Workflows().Create(ctx, domain.WorkflowExecution{
			ID:           wfID,
			WorkflowType: "test-wf",
			TaskQueue:    "default",
			Status:       domain.WorkflowStatusRunning,
		}); err != nil {
			return err
		}
		return errors.New("intentional rollback")
	})
	require.Error(t, err)

	_, err = store.Workflows().Get(ctx, wfID)
	assert.ErrorIs(t, err, domain.ErrWorkflowNotFound)
}

func TestTxManager_RunInTx_MultiRepo(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	wfID := uuid.New()
	err := store.RunInTx(ctx, func(ctx context.Context) error {
		if err := store.Workflows().Create(ctx, domain.WorkflowExecution{
			ID:           wfID,
			WorkflowType: "test-wf",
			TaskQueue:    "default",
			Status:       domain.WorkflowStatusRunning,
			Input:        json.RawMessage(`{}`),
		}); err != nil {
			return err
		}
		if err := store.Events().Append(ctx, []domain.HistoryEvent{{
			WorkflowID: wfID,
			Type:       domain.EventWorkflowExecutionStarted,
			Data:       json.RawMessage(`{}`),
		}}); err != nil {
			return err
		}
		return store.WorkflowTasks().Enqueue(ctx, domain.WorkflowTask{
			QueueName:  "default",
			WorkflowID: wfID,
		})
	})
	require.NoError(t, err)

	wf, err := store.Workflows().Get(ctx, wfID)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkflowStatusRunning, wf.Status)

	events, err := store.Events().GetByWorkflowID(ctx, wfID)
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestTxManager_RunInTx_Nested(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	wfID := uuid.New()
	err := store.RunInTx(ctx, func(ctx context.Context) error {
		if err := store.Workflows().Create(ctx, domain.WorkflowExecution{
			ID:           wfID,
			WorkflowType: "test-wf",
			TaskQueue:    "default",
			Status:       domain.WorkflowStatusRunning,
		}); err != nil {
			return err
		}
		// Nested RunInTx reuses the same transaction
		return store.RunInTx(ctx, func(ctx context.Context) error {
			return store.Events().Append(ctx, []domain.HistoryEvent{{
				WorkflowID: wfID,
				Type:       domain.EventWorkflowExecutionStarted,
				Data:       json.RawMessage(`{}`),
			}})
		})
	})
	require.NoError(t, err)

	events, err := store.Events().GetByWorkflowID(ctx, wfID)
	require.NoError(t, err)
	assert.Len(t, events, 1)
}
