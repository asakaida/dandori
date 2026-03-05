package postgres_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asakaida/dandori/internal/domain"
)

func TestWorkflowStore_Create_Get(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	wfID := uuid.New()
	input := json.RawMessage(`{"order_id":123}`)
	err := store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID:           wfID,
		WorkflowType: "order-workflow",
		TaskQueue:    "default",
		Status:       domain.WorkflowStatusRunning,
		Input:        input,
	})
	require.NoError(t, err)

	wf, err := store.Workflows().Get(ctx, wfID)
	require.NoError(t, err)
	assert.Equal(t, wfID, wf.ID)
	assert.Equal(t, "order-workflow", wf.WorkflowType)
	assert.Equal(t, "default", wf.TaskQueue)
	assert.Equal(t, domain.WorkflowStatusRunning, wf.Status)
	assert.JSONEq(t, `{"order_id":123}`, string(wf.Input))
	assert.Nil(t, wf.ClosedAt)
}

func TestWorkflowStore_Get_NotFound(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	_, err := store.Workflows().Get(ctx, uuid.New())
	assert.ErrorIs(t, err, domain.ErrWorkflowNotFound)
}

func TestWorkflowStore_UpdateStatus(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	wfID := uuid.New()
	require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID:           wfID,
		WorkflowType: "test-wf",
		TaskQueue:    "default",
		Status:       domain.WorkflowStatusRunning,
	}))

	result := json.RawMessage(`{"output":"done"}`)
	err := store.Workflows().UpdateStatus(ctx, wfID, domain.WorkflowStatusCompleted, result, "")
	require.NoError(t, err)

	wf, err := store.Workflows().Get(ctx, wfID)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkflowStatusCompleted, wf.Status)
	assert.JSONEq(t, `{"output":"done"}`, string(wf.Result))
	assert.NotNil(t, wf.ClosedAt)
}

func TestWorkflowStore_Create_UpsertTerminal(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	wfID := uuid.New()
	require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID:           wfID,
		WorkflowType: "old-wf",
		TaskQueue:    "default",
		Status:       domain.WorkflowStatusRunning,
	}))
	require.NoError(t, store.Workflows().UpdateStatus(ctx, wfID, domain.WorkflowStatusCompleted, json.RawMessage(`{"old":true}`), ""))

	// Re-create with same ID should succeed since it's terminal
	err := store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID:           wfID,
		WorkflowType: "new-wf",
		TaskQueue:    "new-queue",
		Status:       domain.WorkflowStatusRunning,
		Input:        json.RawMessage(`{"new":true}`),
	})
	require.NoError(t, err)

	wf, err := store.Workflows().Get(ctx, wfID)
	require.NoError(t, err)
	assert.Equal(t, "new-wf", wf.WorkflowType)
	assert.Equal(t, "new-queue", wf.TaskQueue)
	assert.Equal(t, domain.WorkflowStatusRunning, wf.Status)
	assert.Nil(t, wf.ClosedAt)
	assert.Nil(t, wf.Result)
}

func TestWorkflowStore_Create_DuplicateRunning(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	wfID := uuid.New()
	require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID:           wfID,
		WorkflowType: "test-wf",
		TaskQueue:    "default",
		Status:       domain.WorkflowStatusRunning,
	}))

	// Creating with same ID while RUNNING should fail
	err := store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID:           wfID,
		WorkflowType: "test-wf",
		TaskQueue:    "default",
		Status:       domain.WorkflowStatusRunning,
	})
	assert.ErrorIs(t, err, domain.ErrWorkflowAlreadyExists)
}

func TestWorkflowStore_UpdateStatus_Failed(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	wfID := uuid.New()
	require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID:           wfID,
		WorkflowType: "test-wf",
		TaskQueue:    "default",
		Status:       domain.WorkflowStatusRunning,
	}))

	err := store.Workflows().UpdateStatus(ctx, wfID, domain.WorkflowStatusFailed, nil, "something went wrong")
	require.NoError(t, err)

	wf, err := store.Workflows().Get(ctx, wfID)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkflowStatusFailed, wf.Status)
	assert.Equal(t, "something went wrong", wf.Error)
	assert.NotNil(t, wf.ClosedAt)
}
