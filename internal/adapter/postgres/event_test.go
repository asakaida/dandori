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

func setupWorkflow(t *testing.T, ctx context.Context, wfRepo interface{ Create(context.Context, domain.WorkflowExecution) error }) uuid.UUID {
	t.Helper()
	wfID := uuid.New()
	err := wfRepo.Create(ctx, domain.WorkflowExecution{
		ID:           wfID,
		Namespace:    "default",
		WorkflowType: "test-wf",
		TaskQueue:    "default",
		Status:       domain.WorkflowStatusRunning,
	})
	require.NoError(t, err)
	return wfID
}

func TestEventStore_Append_Single(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	err := store.Events().Append(ctx, []domain.HistoryEvent{{
		WorkflowID: wfID,
		Type:       domain.EventWorkflowExecutionStarted,
		Data:       json.RawMessage(`{"input":"hello"}`),
	}})
	require.NoError(t, err)

	events, err := store.Events().GetByWorkflowID(ctx, wfID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, 1, events[0].SequenceNum)
	assert.Equal(t, domain.EventWorkflowExecutionStarted, events[0].Type)
	assert.JSONEq(t, `{"input":"hello"}`, string(events[0].Data))
}

func TestEventStore_Append_Multiple(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	err := store.Events().Append(ctx, []domain.HistoryEvent{
		{WorkflowID: wfID, Type: domain.EventWorkflowExecutionStarted, Data: json.RawMessage(`{}`)},
		{WorkflowID: wfID, Type: domain.EventActivityTaskScheduled, Data: json.RawMessage(`{"seq_id":0}`)},
	})
	require.NoError(t, err)

	events, err := store.Events().GetByWorkflowID(ctx, wfID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, 1, events[0].SequenceNum)
	assert.Equal(t, 2, events[1].SequenceNum)
}

func TestEventStore_Append_Sequential(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	err := store.Events().Append(ctx, []domain.HistoryEvent{
		{WorkflowID: wfID, Type: domain.EventWorkflowExecutionStarted, Data: json.RawMessage(`{}`)},
	})
	require.NoError(t, err)

	err = store.Events().Append(ctx, []domain.HistoryEvent{
		{WorkflowID: wfID, Type: domain.EventActivityTaskScheduled, Data: json.RawMessage(`{}`)},
	})
	require.NoError(t, err)

	events, err := store.Events().GetByWorkflowID(ctx, wfID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, 1, events[0].SequenceNum)
	assert.Equal(t, 2, events[1].SequenceNum)
}

func TestEventStore_DeleteByWorkflowID(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	require.NoError(t, store.Events().Append(ctx, []domain.HistoryEvent{
		{WorkflowID: wfID, Type: domain.EventWorkflowExecutionStarted, Data: json.RawMessage(`{}`)},
		{WorkflowID: wfID, Type: domain.EventActivityTaskScheduled, Data: json.RawMessage(`{}`)},
	}))

	err := store.Events().DeleteByWorkflowID(ctx, wfID)
	require.NoError(t, err)

	events, err := store.Events().GetByWorkflowID(ctx, wfID)
	require.NoError(t, err)
	assert.Len(t, events, 0)
}

func TestEventStore_GetByWorkflowID_OrderBySequence(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	events := []domain.HistoryEvent{
		{WorkflowID: wfID, Type: domain.EventWorkflowExecutionStarted, Data: json.RawMessage(`{}`)},
		{WorkflowID: wfID, Type: domain.EventActivityTaskScheduled, Data: json.RawMessage(`{}`)},
		{WorkflowID: wfID, Type: domain.EventActivityTaskCompleted, Data: json.RawMessage(`{}`)},
	}
	require.NoError(t, store.Events().Append(ctx, events))

	result, err := store.Events().GetByWorkflowID(ctx, wfID)
	require.NoError(t, err)
	require.Len(t, result, 3)
	for i, e := range result {
		assert.Equal(t, i+1, e.SequenceNum)
	}
}
