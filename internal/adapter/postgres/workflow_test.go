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
