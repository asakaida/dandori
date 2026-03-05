package engine

import (
	"context"
	"testing"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackgroundWorker_CheckActivityTimeouts(t *testing.T) {
	wfID := uuid.New()
	var completedTaskID int64
	var appendedEvents []domain.HistoryEvent
	var enqueuedTask domain.WorkflowTask

	w := NewBackgroundWorker(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{
			AppendFn: func(_ context.Context, events []domain.HistoryEvent) error {
				appendedEvents = events
				return nil
			},
		},
		&mockWorkflowTaskRepo{
			EnqueueFn: func(_ context.Context, task domain.WorkflowTask) error {
				enqueuedTask = task
				return nil
			},
		},
		&mockActivityTaskRepo{
			GetTimedOutFn: func(_ context.Context) ([]domain.ActivityTask, error) {
				return []domain.ActivityTask{
					{ID: 10, WorkflowID: wfID, ActivitySeqID: 1},
				}, nil
			},
			CompleteFn: func(_ context.Context, taskID int64) error {
				completedTaskID = taskID
				return nil
			},
		},
		&mockTxManager{},
	)

	err := w.checkActivityTimeouts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(10), completedTaskID)
	require.Len(t, appendedEvents, 1)
	assert.Equal(t, domain.EventActivityTaskTimedOut, appendedEvents[0].Type)
	assert.Equal(t, wfID, enqueuedTask.WorkflowID)
}

func TestBackgroundWorker_CheckActivityTimeouts_TerminalWorkflow(t *testing.T) {
	wfID := uuid.New()
	var completedTaskID int64
	var eventAppended bool

	w := NewBackgroundWorker(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, Status: domain.WorkflowStatusCompleted}, nil
			},
		},
		&mockEventRepo{
			AppendFn: func(_ context.Context, _ []domain.HistoryEvent) error {
				eventAppended = true
				return nil
			},
		},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{
			GetTimedOutFn: func(_ context.Context) ([]domain.ActivityTask, error) {
				return []domain.ActivityTask{
					{ID: 20, WorkflowID: wfID},
				}, nil
			},
			CompleteFn: func(_ context.Context, taskID int64) error {
				completedTaskID = taskID
				return nil
			},
		},
		&mockTxManager{},
	)

	err := w.checkActivityTimeouts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(20), completedTaskID)
	assert.False(t, eventAppended)
}

func TestBackgroundWorker_CheckActivityTimeouts_NoTimedOut(t *testing.T) {
	w := NewBackgroundWorker(
		&mockWorkflowRepo{},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{
			GetTimedOutFn: func(_ context.Context) ([]domain.ActivityTask, error) {
				return nil, nil
			},
		},
		&mockTxManager{},
	)

	err := w.checkActivityTimeouts(context.Background())
	require.NoError(t, err)
}
