package postgres_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/asakaida/dandori/internal/port"
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

func TestWorkflowStore_List_Basic(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
			ID:           uuid.New(),
			WorkflowType: "test-wf",
			TaskQueue:    "default",
			Status:       domain.WorkflowStatusRunning,
		}))
		time.Sleep(10 * time.Millisecond) // ensure distinct created_at
	}

	workflows, err := store.Workflows().List(ctx, port.ListWorkflowsParams{PageSize: 10})
	require.NoError(t, err)
	assert.Len(t, workflows, 3)
}

func TestWorkflowStore_List_Pagination(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ids := make([]uuid.UUID, 5)
	for i := range ids {
		ids[i] = uuid.New()
		require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
			ID:           ids[i],
			WorkflowType: "test-wf",
			TaskQueue:    "default",
			Status:       domain.WorkflowStatusRunning,
		}))
		time.Sleep(10 * time.Millisecond)
	}

	// First page: 3 items
	page1, err := store.Workflows().List(ctx, port.ListWorkflowsParams{PageSize: 3})
	require.NoError(t, err)
	assert.Len(t, page1, 3)

	// Second page using cursor from last item of page1
	cursor := &port.ListWorkflowsCursor{
		CreatedAt: page1[2].CreatedAt,
		ID:        page1[2].ID,
	}
	page2, err := store.Workflows().List(ctx, port.ListWorkflowsParams{
		PageSize: 3,
		Cursor:   cursor,
	})
	require.NoError(t, err)
	assert.Len(t, page2, 2)

	// No overlap
	for _, wf1 := range page1 {
		for _, wf2 := range page2 {
			assert.NotEqual(t, wf1.ID, wf2.ID)
		}
	}
}

func TestWorkflowStore_List_StatusFilter(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	wfRunning := uuid.New()
	require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID: wfRunning, WorkflowType: "wf", TaskQueue: "q", Status: domain.WorkflowStatusRunning,
	}))
	wfCompleted := uuid.New()
	require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID: wfCompleted, WorkflowType: "wf", TaskQueue: "q", Status: domain.WorkflowStatusRunning,
	}))
	require.NoError(t, store.Workflows().UpdateStatus(ctx, wfCompleted, domain.WorkflowStatusCompleted, nil, ""))

	workflows, err := store.Workflows().List(ctx, port.ListWorkflowsParams{
		PageSize:     10,
		StatusFilter: "RUNNING",
	})
	require.NoError(t, err)
	assert.Len(t, workflows, 1)
	assert.Equal(t, wfRunning, workflows[0].ID)
}

func TestWorkflowStore_List_TypeFilter(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID: uuid.New(), WorkflowType: "type-A", TaskQueue: "q", Status: domain.WorkflowStatusRunning,
	}))
	require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID: uuid.New(), WorkflowType: "type-B", TaskQueue: "q", Status: domain.WorkflowStatusRunning,
	}))

	workflows, err := store.Workflows().List(ctx, port.ListWorkflowsParams{
		PageSize:   10,
		TypeFilter: "type-A",
	})
	require.NoError(t, err)
	assert.Len(t, workflows, 1)
	assert.Equal(t, "type-A", workflows[0].WorkflowType)
}

func TestWorkflowStore_List_QueueFilter(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID: uuid.New(), WorkflowType: "wf", TaskQueue: "queue-A", Status: domain.WorkflowStatusRunning,
	}))
	require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID: uuid.New(), WorkflowType: "wf", TaskQueue: "queue-B", Status: domain.WorkflowStatusRunning,
	}))

	workflows, err := store.Workflows().List(ctx, port.ListWorkflowsParams{
		PageSize:    10,
		QueueFilter: "queue-B",
	})
	require.NoError(t, err)
	assert.Len(t, workflows, 1)
	assert.Equal(t, "queue-B", workflows[0].TaskQueue)
}

func TestWorkflowStore_List_MultipleFilters(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID: uuid.New(), WorkflowType: "type-A", TaskQueue: "q1", Status: domain.WorkflowStatusRunning,
	}))
	require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID: uuid.New(), WorkflowType: "type-A", TaskQueue: "q2", Status: domain.WorkflowStatusRunning,
	}))
	require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
		ID: uuid.New(), WorkflowType: "type-B", TaskQueue: "q1", Status: domain.WorkflowStatusRunning,
	}))

	workflows, err := store.Workflows().List(ctx, port.ListWorkflowsParams{
		PageSize:    10,
		TypeFilter:  "type-A",
		QueueFilter: "q1",
	})
	require.NoError(t, err)
	assert.Len(t, workflows, 1)
	assert.Equal(t, "type-A", workflows[0].WorkflowType)
	assert.Equal(t, "q1", workflows[0].TaskQueue)
}

func TestWorkflowStore_List_OrderDesc(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		require.NoError(t, store.Workflows().Create(ctx, domain.WorkflowExecution{
			ID:           uuid.New(),
			WorkflowType: "wf",
			TaskQueue:    "q",
			Status:       domain.WorkflowStatusRunning,
		}))
		time.Sleep(10 * time.Millisecond)
	}

	workflows, err := store.Workflows().List(ctx, port.ListWorkflowsParams{PageSize: 10})
	require.NoError(t, err)
	require.Len(t, workflows, 3)
	// Should be DESC by created_at
	assert.True(t, workflows[0].CreatedAt.After(workflows[1].CreatedAt) || workflows[0].CreatedAt.Equal(workflows[1].CreatedAt))
	assert.True(t, workflows[1].CreatedAt.After(workflows[2].CreatedAt) || workflows[1].CreatedAt.Equal(workflows[2].CreatedAt))
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
