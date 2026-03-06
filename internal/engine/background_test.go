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
		&mockTimerRepo{},
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
		&mockTimerRepo{},
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
		&mockTimerRepo{},
		&mockTxManager{},
	)

	err := w.checkActivityTimeouts(context.Background())
	require.NoError(t, err)
}

func TestBackgroundWorker_CheckHeartbeatTimeouts(t *testing.T) {
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
			GetHeartbeatTimedOutFn: func(_ context.Context) ([]domain.ActivityTask, error) {
				return []domain.ActivityTask{
					{ID: 30, WorkflowID: wfID, ActivitySeqID: 2},
				}, nil
			},
			CompleteFn: func(_ context.Context, taskID int64) error {
				completedTaskID = taskID
				return nil
			},
		},
		&mockTimerRepo{},
		&mockTxManager{},
	)

	err := w.checkHeartbeatTimeouts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(30), completedTaskID)
	require.Len(t, appendedEvents, 1)
	assert.Equal(t, domain.EventActivityTaskTimedOut, appendedEvents[0].Type)
	assert.Equal(t, wfID, enqueuedTask.WorkflowID)
}

func TestBackgroundWorker_PollFiredTimers(t *testing.T) {
	wfID := uuid.New()
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
		&mockActivityTaskRepo{},
		&mockTimerRepo{
			GetFiredFn: func(_ context.Context) ([]domain.Timer, error) {
				return []domain.Timer{
					{ID: 100, WorkflowID: wfID, SeqID: 1},
				}, nil
			},
			MarkFiredFn: func(_ context.Context, _ int64) (bool, error) { return true, nil },
		},
		&mockTxManager{},
	)

	err := w.pollFiredTimers(context.Background())
	require.NoError(t, err)
	require.Len(t, appendedEvents, 1)
	assert.Equal(t, domain.EventTimerFired, appendedEvents[0].Type)
	assert.Equal(t, wfID, enqueuedTask.WorkflowID)
	assert.Equal(t, "default", enqueuedTask.QueueName)
}

func TestBackgroundWorker_PollFiredTimers_AlreadyFired(t *testing.T) {
	wfID := uuid.New()
	var eventAppended bool

	w := NewBackgroundWorker(
		&mockWorkflowRepo{},
		&mockEventRepo{
			AppendFn: func(_ context.Context, _ []domain.HistoryEvent) error {
				eventAppended = true
				return nil
			},
		},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{
			GetFiredFn: func(_ context.Context) ([]domain.Timer, error) {
				return []domain.Timer{
					{ID: 100, WorkflowID: wfID, SeqID: 1},
				}, nil
			},
			MarkFiredFn: func(_ context.Context, _ int64) (bool, error) { return false, nil },
		},
		&mockTxManager{},
	)

	err := w.pollFiredTimers(context.Background())
	require.NoError(t, err)
	assert.False(t, eventAppended)
}

func TestBackgroundWorker_PollFiredTimers_TerminalWorkflow(t *testing.T) {
	wfID := uuid.New()
	var wfTaskEnqueued bool

	w := NewBackgroundWorker(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, Status: domain.WorkflowStatusCompleted}, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{
			EnqueueFn: func(_ context.Context, _ domain.WorkflowTask) error {
				wfTaskEnqueued = true
				return nil
			},
		},
		&mockActivityTaskRepo{},
		&mockTimerRepo{
			GetFiredFn: func(_ context.Context) ([]domain.Timer, error) {
				return []domain.Timer{
					{ID: 100, WorkflowID: wfID, SeqID: 1},
				}, nil
			},
			MarkFiredFn: func(_ context.Context, _ int64) (bool, error) { return true, nil },
		},
		&mockTxManager{},
	)

	err := w.pollFiredTimers(context.Background())
	require.NoError(t, err)
	assert.False(t, wfTaskEnqueued)
}
