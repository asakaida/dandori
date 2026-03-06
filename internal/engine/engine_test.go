package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/asakaida/dandori/internal/port"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- StartWorkflow ---

func TestStartWorkflow_NewWorkflow(t *testing.T) {
	var createdWF domain.WorkflowExecution
	var appendedEvents []domain.HistoryEvent
	var enqueuedTask domain.WorkflowTask

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn:    func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) { return nil, domain.ErrWorkflowNotFound },
			CreateFn: func(_ context.Context, wf domain.WorkflowExecution) error { createdWF = wf; return nil },
		},
		&mockEventRepo{
			AppendFn: func(_ context.Context, events []domain.HistoryEvent) error { appendedEvents = events; return nil },
		},
		&mockWorkflowTaskRepo{
			EnqueueFn: func(_ context.Context, task domain.WorkflowTask) error { enqueuedTask = task; return nil },
		},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	wfID := uuid.New()
	wf, err := e.StartWorkflow(context.Background(), port.StartWorkflowParams{
		ID:           wfID,
		WorkflowType: "test-wf",
		TaskQueue:    "default",
		Input:        json.RawMessage(`{"key":"value"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, wfID, wf.ID)
	assert.Equal(t, domain.WorkflowStatusRunning, wf.Status)
	assert.Equal(t, "test-wf", createdWF.WorkflowType)
	assert.Len(t, appendedEvents, 1)
	assert.Equal(t, domain.EventWorkflowExecutionStarted, appendedEvents[0].Type)
	assert.Equal(t, wfID, enqueuedTask.WorkflowID)
	assert.Equal(t, "default", enqueuedTask.QueueName)
}

func TestStartWorkflow_AutoGenerateID(t *testing.T) {
	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn:    func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) { return nil, domain.ErrWorkflowNotFound },
			CreateFn: func(_ context.Context, _ domain.WorkflowExecution) error { return nil },
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	wf, err := e.StartWorkflow(context.Background(), port.StartWorkflowParams{
		WorkflowType: "test-wf",
		TaskQueue:    "default",
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, wf.ID)
}

func TestStartWorkflow_AlreadyRunning(t *testing.T) {
	wfID := uuid.New()
	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	_, err := e.StartWorkflow(context.Background(), port.StartWorkflowParams{
		ID:           wfID,
		WorkflowType: "test-wf",
		TaskQueue:    "default",
	})
	assert.ErrorIs(t, err, domain.ErrWorkflowAlreadyExists)
}

func TestStartWorkflow_RecreateTerminal(t *testing.T) {
	wfID := uuid.New()
	var deletedEvents, deletedWFTasks, deletedATasks, deletedTimers bool

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, Status: domain.WorkflowStatusCompleted}, nil
			},
			CreateFn: func(_ context.Context, _ domain.WorkflowExecution) error { return nil },
		},
		&mockEventRepo{
			DeleteByWorkflowIDFn: func(_ context.Context, _ uuid.UUID) error { deletedEvents = true; return nil },
		},
		&mockWorkflowTaskRepo{
			DeleteByWorkflowIDFn: func(_ context.Context, _ uuid.UUID) error { deletedWFTasks = true; return nil },
		},
		&mockActivityTaskRepo{
			DeleteByWorkflowIDFn: func(_ context.Context, _ uuid.UUID) error { deletedATasks = true; return nil },
		},
		&mockTimerRepo{
			DeleteByWorkflowIDFn: func(_ context.Context, _ uuid.UUID) error { deletedTimers = true; return nil },
		},
	)

	wf, err := e.StartWorkflow(context.Background(), port.StartWorkflowParams{
		ID:           wfID,
		WorkflowType: "test-wf",
		TaskQueue:    "default",
	})
	require.NoError(t, err)
	assert.Equal(t, wfID, wf.ID)
	assert.True(t, deletedEvents)
	assert.True(t, deletedWFTasks)
	assert.True(t, deletedATasks)
	assert.True(t, deletedTimers)
}

// --- DescribeWorkflow ---

func TestDescribeWorkflow(t *testing.T) {
	wfID := uuid.New()
	expected := &domain.WorkflowExecution{ID: wfID, Status: domain.WorkflowStatusRunning}
	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) { return expected, nil },
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	wf, err := e.DescribeWorkflow(context.Background(), wfID)
	require.NoError(t, err)
	assert.Equal(t, expected, wf)
}

func TestDescribeWorkflow_NotFound(t *testing.T) {
	e := newTestEngine(
		&mockWorkflowRepo{},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	_, err := e.DescribeWorkflow(context.Background(), uuid.New())
	assert.ErrorIs(t, err, domain.ErrWorkflowNotFound)
}

// --- GetWorkflowHistory ---

func TestGetWorkflowHistory(t *testing.T) {
	wfID := uuid.New()
	expected := []domain.HistoryEvent{{ID: 1, WorkflowID: wfID, Type: domain.EventWorkflowExecutionStarted}}
	e := newTestEngine(
		&mockWorkflowRepo{},
		&mockEventRepo{
			GetByWorkflowIDFn: func(_ context.Context, _ uuid.UUID) ([]domain.HistoryEvent, error) { return expected, nil },
		},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	events, err := e.GetWorkflowHistory(context.Background(), wfID)
	require.NoError(t, err)
	assert.Equal(t, expected, events)
}

// --- TerminateWorkflow ---

func TestTerminateWorkflow(t *testing.T) {
	wfID := uuid.New()
	var updatedStatus domain.WorkflowStatus
	var appendedEvents []domain.HistoryEvent

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, Status: domain.WorkflowStatusRunning}, nil
			},
			UpdateStatusFn: func(_ context.Context, _ uuid.UUID, status domain.WorkflowStatus, _ json.RawMessage, _ string) error {
				updatedStatus = status
				return nil
			},
		},
		&mockEventRepo{
			AppendFn: func(_ context.Context, events []domain.HistoryEvent) error { appendedEvents = events; return nil },
		},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	err := e.TerminateWorkflow(context.Background(), wfID, "test reason")
	require.NoError(t, err)
	assert.Equal(t, domain.WorkflowStatusTerminated, updatedStatus)
	require.Len(t, appendedEvents, 1)
	assert.Equal(t, domain.EventWorkflowExecutionTerminated, appendedEvents[0].Type)
}

func TestTerminateWorkflow_NotRunning(t *testing.T) {
	wfID := uuid.New()
	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, Status: domain.WorkflowStatusCompleted}, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	err := e.TerminateWorkflow(context.Background(), wfID, "reason")
	assert.ErrorIs(t, err, domain.ErrWorkflowNotRunning)
}

// --- SignalWorkflow ---

func TestSignalWorkflow(t *testing.T) {
	wfID := uuid.New()
	var appendedEvents []domain.HistoryEvent
	var enqueuedTask domain.WorkflowTask

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{
			AppendFn: func(_ context.Context, events []domain.HistoryEvent) error { appendedEvents = events; return nil },
		},
		&mockWorkflowTaskRepo{
			EnqueueFn: func(_ context.Context, task domain.WorkflowTask) error { enqueuedTask = task; return nil },
		},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	err := e.SignalWorkflow(context.Background(), wfID, "approval", json.RawMessage(`{"approved":true}`))
	require.NoError(t, err)
	require.Len(t, appendedEvents, 1)
	assert.Equal(t, domain.EventWorkflowSignaled, appendedEvents[0].Type)
	assert.Equal(t, wfID, appendedEvents[0].WorkflowID)
	assert.Equal(t, wfID, enqueuedTask.WorkflowID)
	assert.Equal(t, "default", enqueuedTask.QueueName)
}

func TestSignalWorkflow_NotRunning(t *testing.T) {
	wfID := uuid.New()
	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, Status: domain.WorkflowStatusCompleted}, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	err := e.SignalWorkflow(context.Background(), wfID, "approval", json.RawMessage(`{}`))
	assert.ErrorIs(t, err, domain.ErrWorkflowNotRunning)
}

// --- CancelWorkflow ---

func TestCancelWorkflow(t *testing.T) {
	wfID := uuid.New()
	var appendedEvents []domain.HistoryEvent
	var enqueuedTask domain.WorkflowTask

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{
			AppendFn: func(_ context.Context, events []domain.HistoryEvent) error { appendedEvents = events; return nil },
		},
		&mockWorkflowTaskRepo{
			EnqueueFn: func(_ context.Context, task domain.WorkflowTask) error { enqueuedTask = task; return nil },
		},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	err := e.CancelWorkflow(context.Background(), wfID)
	require.NoError(t, err)
	require.Len(t, appendedEvents, 1)
	assert.Equal(t, domain.EventWorkflowCancelRequested, appendedEvents[0].Type)
	assert.Equal(t, wfID, appendedEvents[0].WorkflowID)
	assert.Equal(t, wfID, enqueuedTask.WorkflowID)
	assert.Equal(t, "default", enqueuedTask.QueueName)
}

func TestCancelWorkflow_NotRunning(t *testing.T) {
	wfID := uuid.New()
	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, Status: domain.WorkflowStatusCompleted}, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	err := e.CancelWorkflow(context.Background(), wfID)
	assert.ErrorIs(t, err, domain.ErrWorkflowNotRunning)
}

func TestSignalWorkflow_MultipleSignals(t *testing.T) {
	wfID := uuid.New()
	var appendCount int
	var enqueueCount int

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{
			AppendFn: func(_ context.Context, _ []domain.HistoryEvent) error { appendCount++; return nil },
		},
		&mockWorkflowTaskRepo{
			EnqueueFn: func(_ context.Context, _ domain.WorkflowTask) error { enqueueCount++; return nil },
		},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	for i := 0; i < 3; i++ {
		err := e.SignalWorkflow(context.Background(), wfID, "signal", json.RawMessage(`{}`))
		require.NoError(t, err)
	}
	assert.Equal(t, 3, appendCount)
	assert.Equal(t, 3, enqueueCount)
}

// --- PollWorkflowTask ---

func TestPollWorkflowTask_NoTask(t *testing.T) {
	e := newTestEngine(
		&mockWorkflowRepo{},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	result, err := e.PollWorkflowTask(context.Background(), "default", "worker-1")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestPollWorkflowTask_WithTask(t *testing.T) {
	wfID := uuid.New()
	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, WorkflowType: "OrderWorkflow", Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{
			GetByWorkflowIDFn: func(_ context.Context, _ uuid.UUID) ([]domain.HistoryEvent, error) {
				return []domain.HistoryEvent{{ID: 1, WorkflowID: wfID}}, nil
			},
		},
		&mockWorkflowTaskRepo{
			PollFn: func(_ context.Context, _ string, _ string) (*domain.WorkflowTask, error) {
				return &domain.WorkflowTask{ID: 1, WorkflowID: wfID}, nil
			},
		},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	result, err := e.PollWorkflowTask(context.Background(), "default", "worker-1")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int64(1), result.Task.ID)
	assert.Len(t, result.Events, 1)
	assert.Equal(t, "OrderWorkflow", result.WorkflowType)
}

// --- CompleteWorkflowTask ---

func TestCompleteWorkflowTask(t *testing.T) {
	wfID := uuid.New()
	var completedTaskID int64

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusRunning}, nil
			},
			UpdateStatusFn: func(_ context.Context, _ uuid.UUID, _ domain.WorkflowStatus, _ json.RawMessage, _ string) error { return nil },
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{
			GetByIDFn: func(_ context.Context, taskID int64) (*domain.WorkflowTask, error) {
				return &domain.WorkflowTask{ID: taskID, WorkflowID: wfID}, nil
			},
			CompleteFn: func(_ context.Context, taskID int64) error { completedTaskID = taskID; return nil },
		},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	commands := []domain.Command{
		{
			Type:       domain.CommandCompleteWorkflow,
			Attributes: json.RawMessage(`{"result":{"done":true}}`),
		},
	}
	err := e.CompleteWorkflowTask(context.Background(), 42, commands)
	require.NoError(t, err)
	assert.Equal(t, int64(42), completedTaskID)
}

// --- FailWorkflowTask ---

func TestFailWorkflowTask(t *testing.T) {
	wfID := uuid.New()
	var updatedStatus domain.WorkflowStatus

	e := newTestEngine(
		&mockWorkflowRepo{
			UpdateStatusFn: func(_ context.Context, _ uuid.UUID, status domain.WorkflowStatus, _ json.RawMessage, _ string) error {
				updatedStatus = status
				return nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{
			GetByIDFn: func(_ context.Context, _ int64) (*domain.WorkflowTask, error) {
				return &domain.WorkflowTask{ID: 1, WorkflowID: wfID}, nil
			},
			CompleteFn: func(_ context.Context, _ int64) error { return nil },
		},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	err := e.FailWorkflowTask(context.Background(), 1, "panic", "something broke")
	require.NoError(t, err)
	assert.Equal(t, domain.WorkflowStatusFailed, updatedStatus)
}

// --- PollActivityTask ---

func TestPollActivityTask_NoTask(t *testing.T) {
	e := newTestEngine(
		&mockWorkflowRepo{},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	task, err := e.PollActivityTask(context.Background(), "default", "worker-1")
	require.NoError(t, err)
	assert.Nil(t, task)
}

func TestPollActivityTask_WithTask(t *testing.T) {
	wfID := uuid.New()
	e := newTestEngine(
		&mockWorkflowRepo{},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{
			PollFn: func(_ context.Context, _ string, _ string) (*domain.ActivityTask, error) {
				return &domain.ActivityTask{ID: 1, WorkflowID: wfID, ActivityType: "send-email"}, nil
			},
		},
		&mockTimerRepo{},
	)

	task, err := e.PollActivityTask(context.Background(), "default", "worker-1")
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, "send-email", task.ActivityType)
}

// --- CompleteActivityTask ---

func TestCompleteActivityTask(t *testing.T) {
	wfID := uuid.New()
	var enqueuedWFTask domain.WorkflowTask
	var appendedEvents []domain.HistoryEvent

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{
			AppendFn: func(_ context.Context, events []domain.HistoryEvent) error { appendedEvents = events; return nil },
		},
		&mockWorkflowTaskRepo{
			EnqueueFn: func(_ context.Context, task domain.WorkflowTask) error { enqueuedWFTask = task; return nil },
		},
		&mockActivityTaskRepo{
			GetByIDFn: func(_ context.Context, _ int64) (*domain.ActivityTask, error) {
				return &domain.ActivityTask{ID: 1, WorkflowID: wfID, ActivitySeqID: 5}, nil
			},
			CompleteFn: func(_ context.Context, _ int64) error { return nil },
		},
		&mockTimerRepo{},
	)

	err := e.CompleteActivityTask(context.Background(), 1, json.RawMessage(`{"ok":true}`))
	require.NoError(t, err)
	assert.Equal(t, wfID, enqueuedWFTask.WorkflowID)
	assert.Equal(t, "default", enqueuedWFTask.QueueName)
	require.Len(t, appendedEvents, 1)
	assert.Equal(t, domain.EventActivityTaskCompleted, appendedEvents[0].Type)
}

func TestCompleteActivityTask_TerminalWorkflow(t *testing.T) {
	wfID := uuid.New()
	var wfTaskEnqueued bool

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, Status: domain.WorkflowStatusCompleted}, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{
			EnqueueFn: func(_ context.Context, _ domain.WorkflowTask) error { wfTaskEnqueued = true; return nil },
		},
		&mockActivityTaskRepo{
			GetByIDFn: func(_ context.Context, _ int64) (*domain.ActivityTask, error) {
				return &domain.ActivityTask{ID: 1, WorkflowID: wfID}, nil
			},
			CompleteFn: func(_ context.Context, _ int64) error { return nil },
		},
		&mockTimerRepo{},
	)

	err := e.CompleteActivityTask(context.Background(), 1, json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.False(t, wfTaskEnqueued)
}

// --- FailActivityTask ---

func TestFailActivityTask_NonRetryable(t *testing.T) {
	wfID := uuid.New()
	var appendedEvents []domain.HistoryEvent
	var wfTaskEnqueued bool

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{
			AppendFn: func(_ context.Context, events []domain.HistoryEvent) error { appendedEvents = events; return nil },
		},
		&mockWorkflowTaskRepo{
			EnqueueFn: func(_ context.Context, _ domain.WorkflowTask) error { wfTaskEnqueued = true; return nil },
		},
		&mockActivityTaskRepo{
			GetByIDFn: func(_ context.Context, _ int64) (*domain.ActivityTask, error) {
				return &domain.ActivityTask{ID: 1, WorkflowID: wfID, Attempt: 1, MaxAttempts: 3}, nil
			},
			CompleteFn: func(_ context.Context, _ int64) error { return nil },
		},
		&mockTimerRepo{},
	)

	err := e.FailActivityTask(context.Background(), 1, domain.ActivityFailure{
		Message:      "fatal error",
		NonRetryable: true,
	})
	require.NoError(t, err)
	assert.True(t, wfTaskEnqueued)
	require.Len(t, appendedEvents, 1)
	assert.Equal(t, domain.EventActivityTaskFailed, appendedEvents[0].Type)
}

func TestFailActivityTask_MaxAttemptsReached(t *testing.T) {
	wfID := uuid.New()
	var completed bool

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{
			GetByIDFn: func(_ context.Context, _ int64) (*domain.ActivityTask, error) {
				return &domain.ActivityTask{ID: 1, WorkflowID: wfID, Attempt: 3, MaxAttempts: 3}, nil
			},
			CompleteFn: func(_ context.Context, _ int64) error { completed = true; return nil },
		},
		&mockTimerRepo{},
	)

	err := e.FailActivityTask(context.Background(), 1, domain.ActivityFailure{Message: "error"})
	require.NoError(t, err)
	assert.True(t, completed)
}

func TestFailActivityTask_Retry(t *testing.T) {
	wfID := uuid.New()
	var requeued bool

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{
			GetByIDFn: func(_ context.Context, _ int64) (*domain.ActivityTask, error) {
				return &domain.ActivityTask{ID: 1, WorkflowID: wfID, Attempt: 1, MaxAttempts: 3}, nil
			},
			RequeueFn: func(_ context.Context, _ int64, _ time.Time) error { requeued = true; return nil },
		},
		&mockTimerRepo{},
	)

	err := e.FailActivityTask(context.Background(), 1, domain.ActivityFailure{Message: "transient error"})
	require.NoError(t, err)
	assert.True(t, requeued)
}

func TestFailActivityTask_TerminalWorkflow(t *testing.T) {
	wfID := uuid.New()
	var completed bool

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, Status: domain.WorkflowStatusTerminated}, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{
			GetByIDFn: func(_ context.Context, _ int64) (*domain.ActivityTask, error) {
				return &domain.ActivityTask{ID: 1, WorkflowID: wfID, Attempt: 1, MaxAttempts: 3}, nil
			},
			CompleteFn: func(_ context.Context, _ int64) error { completed = true; return nil },
		},
		&mockTimerRepo{},
	)

	err := e.FailActivityTask(context.Background(), 1, domain.ActivityFailure{Message: "error"})
	require.NoError(t, err)
	assert.True(t, completed)
}

// --- RecordActivityHeartbeat ---

func TestRecordActivityHeartbeat(t *testing.T) {
	var updatedTaskID int64

	e := newTestEngine(
		&mockWorkflowRepo{},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{
			UpdateHeartbeatFn: func(_ context.Context, taskID int64) error {
				updatedTaskID = taskID
				return nil
			},
		},
		&mockTimerRepo{},
	)

	err := e.RecordActivityHeartbeat(context.Background(), 42, json.RawMessage(`{"progress":50}`))
	require.NoError(t, err)
	assert.Equal(t, int64(42), updatedTaskID)
}

// --- ListWorkflows ---

func TestListWorkflows_DefaultPageSize(t *testing.T) {
	var gotParams port.ListWorkflowsParams
	e := newTestEngine(
		&mockWorkflowRepo{
			ListFn: func(_ context.Context, params port.ListWorkflowsParams) ([]domain.WorkflowExecution, error) {
				gotParams = params
				return nil, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	_, err := e.ListWorkflows(context.Background(), port.ListWorkflowsParams{})
	require.NoError(t, err)
	assert.Equal(t, 21, gotParams.PageSize) // default 20 + 1
}

func TestListWorkflows_MaxPageSize(t *testing.T) {
	var gotParams port.ListWorkflowsParams
	e := newTestEngine(
		&mockWorkflowRepo{
			ListFn: func(_ context.Context, params port.ListWorkflowsParams) ([]domain.WorkflowExecution, error) {
				gotParams = params
				return nil, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	_, err := e.ListWorkflows(context.Background(), port.ListWorkflowsParams{PageSize: 200})
	require.NoError(t, err)
	assert.Equal(t, 101, gotParams.PageSize) // capped to 100 + 1
}

func TestListWorkflows_NextCursor(t *testing.T) {
	now := time.Now()
	workflows := make([]domain.WorkflowExecution, 4)
	for i := range workflows {
		workflows[i] = domain.WorkflowExecution{
			ID:        uuid.New(),
			CreatedAt: now.Add(-time.Duration(i) * time.Minute),
		}
	}

	e := newTestEngine(
		&mockWorkflowRepo{
			ListFn: func(_ context.Context, _ port.ListWorkflowsParams) ([]domain.WorkflowExecution, error) {
				return workflows, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	result, err := e.ListWorkflows(context.Background(), port.ListWorkflowsParams{PageSize: 3})
	require.NoError(t, err)
	assert.Len(t, result.Workflows, 3)
	require.NotNil(t, result.NextCursor)
	assert.Equal(t, workflows[2].ID, result.NextCursor.ID)
	assert.Equal(t, workflows[2].CreatedAt, result.NextCursor.CreatedAt)
}

func TestListWorkflows_NoNextCursor(t *testing.T) {
	workflows := []domain.WorkflowExecution{
		{ID: uuid.New(), CreatedAt: time.Now()},
	}

	e := newTestEngine(
		&mockWorkflowRepo{
			ListFn: func(_ context.Context, _ port.ListWorkflowsParams) ([]domain.WorkflowExecution, error) {
				return workflows, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
	)

	result, err := e.ListWorkflows(context.Background(), port.ListWorkflowsParams{PageSize: 3})
	require.NoError(t, err)
	assert.Len(t, result.Workflows, 1)
	assert.Nil(t, result.NextCursor)
}

func TestRecordActivityHeartbeat_TaskNotFound(t *testing.T) {
	e := newTestEngine(
		&mockWorkflowRepo{},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{
			UpdateHeartbeatFn: func(_ context.Context, _ int64) error {
				return domain.ErrTaskNotFound
			},
		},
		&mockTimerRepo{},
	)

	err := e.RecordActivityHeartbeat(context.Background(), 999, nil)
	assert.ErrorIs(t, err, domain.ErrTaskNotFound)
}

// --- ProcessCommands: ScheduleActivity with Schedule Timeouts ---

func TestProcessCommands_ScheduleActivity_WithScheduleToCloseTimeout(t *testing.T) {
	wfID := uuid.New()
	var enqueuedTask domain.ActivityTask

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{
			GetByIDFn: func(_ context.Context, taskID int64) (*domain.WorkflowTask, error) {
				return &domain.WorkflowTask{ID: taskID, WorkflowID: wfID}, nil
			},
			CompleteFn: func(_ context.Context, _ int64) error { return nil },
		},
		&mockActivityTaskRepo{
			EnqueueFn: func(_ context.Context, task domain.ActivityTask) error { enqueuedTask = task; return nil },
		},
		&mockTimerRepo{},
	)

	attrs, _ := json.Marshal(domain.ScheduleActivityTaskAttributes{
		SeqID:                  1,
		ActivityType:           "test",
		Input:                  json.RawMessage(`{}`),
		StartToCloseTimeout:    10 * time.Second,
		ScheduleToCloseTimeout: 30 * time.Second,
	})
	err := e.CompleteWorkflowTask(context.Background(), 1, []domain.Command{
		{Type: domain.CommandScheduleActivityTask, Attributes: attrs},
	})
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, enqueuedTask.ScheduleToCloseTimeout)
	require.NotNil(t, enqueuedTask.ScheduleToCloseTimeoutAt)
	assert.WithinDuration(t, time.Now().Add(30*time.Second), *enqueuedTask.ScheduleToCloseTimeoutAt, 2*time.Second)
	assert.Nil(t, enqueuedTask.ScheduleToStartTimeoutAt)
}

func TestProcessCommands_ScheduleActivity_WithScheduleToStartTimeout(t *testing.T) {
	wfID := uuid.New()
	var enqueuedTask domain.ActivityTask

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{
			GetByIDFn: func(_ context.Context, taskID int64) (*domain.WorkflowTask, error) {
				return &domain.WorkflowTask{ID: taskID, WorkflowID: wfID}, nil
			},
			CompleteFn: func(_ context.Context, _ int64) error { return nil },
		},
		&mockActivityTaskRepo{
			EnqueueFn: func(_ context.Context, task domain.ActivityTask) error { enqueuedTask = task; return nil },
		},
		&mockTimerRepo{},
	)

	attrs, _ := json.Marshal(domain.ScheduleActivityTaskAttributes{
		SeqID:                  1,
		ActivityType:           "test",
		Input:                  json.RawMessage(`{}`),
		StartToCloseTimeout:    10 * time.Second,
		ScheduleToStartTimeout: 5 * time.Second,
	})
	err := e.CompleteWorkflowTask(context.Background(), 1, []domain.Command{
		{Type: domain.CommandScheduleActivityTask, Attributes: attrs},
	})
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, enqueuedTask.ScheduleToStartTimeout)
	require.NotNil(t, enqueuedTask.ScheduleToStartTimeoutAt)
	assert.WithinDuration(t, time.Now().Add(5*time.Second), *enqueuedTask.ScheduleToStartTimeoutAt, 2*time.Second)
	assert.Nil(t, enqueuedTask.ScheduleToCloseTimeoutAt)
}

// --- ProcessCommands: Timer ---

func TestProcessCommands_StartTimer(t *testing.T) {
	wfID := uuid.New()
	var createdTimer domain.Timer
	var appendedEvents []domain.HistoryEvent

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{
			AppendFn: func(_ context.Context, events []domain.HistoryEvent) error { appendedEvents = events; return nil },
		},
		&mockWorkflowTaskRepo{
			GetByIDFn: func(_ context.Context, taskID int64) (*domain.WorkflowTask, error) {
				return &domain.WorkflowTask{ID: taskID, WorkflowID: wfID}, nil
			},
			CompleteFn: func(_ context.Context, _ int64) error { return nil },
		},
		&mockActivityTaskRepo{},
		&mockTimerRepo{
			CreateFn: func(_ context.Context, timer domain.Timer) error { createdTimer = timer; return nil },
		},
	)

	attrs, _ := json.Marshal(domain.StartTimerAttributes{SeqID: 1, Duration: 5 * time.Second})
	err := e.CompleteWorkflowTask(context.Background(), 1, []domain.Command{
		{Type: domain.CommandStartTimer, Attributes: attrs},
	})
	require.NoError(t, err)
	assert.Equal(t, wfID, createdTimer.WorkflowID)
	assert.Equal(t, int64(1), createdTimer.SeqID)
	assert.WithinDuration(t, time.Now().Add(5*time.Second), createdTimer.FireAt, 2*time.Second)
	require.Len(t, appendedEvents, 1)
	assert.Equal(t, domain.EventTimerStarted, appendedEvents[0].Type)
}

func TestProcessCommands_CancelTimer_Pending(t *testing.T) {
	wfID := uuid.New()
	var appendedEvents []domain.HistoryEvent

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{
			AppendFn: func(_ context.Context, events []domain.HistoryEvent) error { appendedEvents = events; return nil },
		},
		&mockWorkflowTaskRepo{
			GetByIDFn: func(_ context.Context, taskID int64) (*domain.WorkflowTask, error) {
				return &domain.WorkflowTask{ID: taskID, WorkflowID: wfID}, nil
			},
			CompleteFn: func(_ context.Context, _ int64) error { return nil },
		},
		&mockActivityTaskRepo{},
		&mockTimerRepo{
			CancelFn: func(_ context.Context, _ uuid.UUID, _ int64) (bool, error) { return true, nil },
		},
	)

	attrs, _ := json.Marshal(domain.CancelTimerAttributes{SeqID: 1})
	err := e.CompleteWorkflowTask(context.Background(), 1, []domain.Command{
		{Type: domain.CommandCancelTimer, Attributes: attrs},
	})
	require.NoError(t, err)
	require.Len(t, appendedEvents, 1)
	assert.Equal(t, domain.EventTimerCanceled, appendedEvents[0].Type)
}

func TestProcessCommands_CancelTimer_AlreadyFired(t *testing.T) {
	wfID := uuid.New()
	var eventAppended bool

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusRunning}, nil
			},
		},
		&mockEventRepo{
			AppendFn: func(_ context.Context, _ []domain.HistoryEvent) error { eventAppended = true; return nil },
		},
		&mockWorkflowTaskRepo{
			GetByIDFn: func(_ context.Context, taskID int64) (*domain.WorkflowTask, error) {
				return &domain.WorkflowTask{ID: taskID, WorkflowID: wfID}, nil
			},
			CompleteFn: func(_ context.Context, _ int64) error { return nil },
		},
		&mockActivityTaskRepo{},
		&mockTimerRepo{
			CancelFn: func(_ context.Context, _ uuid.UUID, _ int64) (bool, error) { return false, nil },
		},
	)

	attrs, _ := json.Marshal(domain.CancelTimerAttributes{SeqID: 1})
	err := e.CompleteWorkflowTask(context.Background(), 1, []domain.Command{
		{Type: domain.CommandCancelTimer, Attributes: attrs},
	})
	require.NoError(t, err)
	assert.False(t, eventAppended)
}
