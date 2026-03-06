package postgres_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asakaida/dandori/internal/domain"
)

func TestQueryStore_CreateAndGetByID(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	id, err := store.Queries().Create(ctx, domain.WorkflowQuery{
		WorkflowID: wfID,
		QueryType:  "getState",
		Input:      json.RawMessage(`{"key":"value"}`),
		Status:     domain.QueryStatusPending,
	})
	require.NoError(t, err)
	assert.NotZero(t, id)

	q, err := store.Queries().GetByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, id, q.ID)
	assert.Equal(t, wfID, q.WorkflowID)
	assert.Equal(t, "getState", q.QueryType)
	assert.JSONEq(t, `{"key":"value"}`, string(q.Input))
	assert.Equal(t, domain.QueryStatusPending, q.Status)
	assert.Nil(t, q.AnsweredAt)
}

func TestQueryStore_GetPendingByWorkflowID(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	id1, err := store.Queries().Create(ctx, domain.WorkflowQuery{
		WorkflowID: wfID,
		QueryType:  "q1",
		Status:     domain.QueryStatusPending,
	})
	require.NoError(t, err)

	id2, err := store.Queries().Create(ctx, domain.WorkflowQuery{
		WorkflowID: wfID,
		QueryType:  "q2",
		Status:     domain.QueryStatusPending,
	})
	require.NoError(t, err)

	queries, err := store.Queries().GetPendingByWorkflowID(ctx, wfID)
	require.NoError(t, err)
	require.Len(t, queries, 2)
	assert.Equal(t, id1, queries[0].ID)
	assert.Equal(t, id2, queries[1].ID)
}

func TestQueryStore_SetResult_Success(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	id, err := store.Queries().Create(ctx, domain.WorkflowQuery{
		WorkflowID: wfID,
		QueryType:  "getState",
		Status:     domain.QueryStatusPending,
	})
	require.NoError(t, err)

	err = store.Queries().SetResult(ctx, id, json.RawMessage(`"result"`), "")
	require.NoError(t, err)

	q, err := store.Queries().GetByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.QueryStatusAnswered, q.Status)
	assert.JSONEq(t, `"result"`, string(q.Result))
	assert.NotNil(t, q.AnsweredAt)

	// Should not appear in pending
	pending, err := store.Queries().GetPendingByWorkflowID(ctx, wfID)
	require.NoError(t, err)
	assert.Len(t, pending, 0)
}

func TestQueryStore_SetResult_AlreadyAnswered(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	id, err := store.Queries().Create(ctx, domain.WorkflowQuery{
		WorkflowID: wfID,
		QueryType:  "getState",
		Status:     domain.QueryStatusPending,
	})
	require.NoError(t, err)

	err = store.Queries().SetResult(ctx, id, json.RawMessage(`"result"`), "")
	require.NoError(t, err)

	// Second call should fail
	err = store.Queries().SetResult(ctx, id, json.RawMessage(`"result2"`), "")
	assert.ErrorIs(t, err, domain.ErrQueryNotFound)
}

func TestQueryStore_DeleteByWorkflowID(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wfID := setupWorkflow(t, ctx, store.Workflows())

	_, err := store.Queries().Create(ctx, domain.WorkflowQuery{
		WorkflowID: wfID,
		QueryType:  "q1",
		Status:     domain.QueryStatusPending,
	})
	require.NoError(t, err)

	err = store.Queries().DeleteByWorkflowID(ctx, wfID)
	require.NoError(t, err)

	pending, err := store.Queries().GetPendingByWorkflowID(ctx, wfID)
	require.NoError(t, err)
	assert.Len(t, pending, 0)
}
