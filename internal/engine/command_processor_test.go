package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessCommands_ScheduleActivity_FallbackTaskQueue(t *testing.T) {
	var enqueuedTask domain.ActivityTask

	e := newTestEngine(
		&mockWorkflowRepo{},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{
			EnqueueFn: func(_ context.Context, task domain.ActivityTask) error {
				enqueuedTask = task
				return nil
			},
		},
		&mockTimerRepo{},
		&mockQueryRepo{},
	)

	wfID := uuid.New()
	attrs, _ := json.Marshal(domain.ScheduleActivityTaskAttributes{
		SeqID:        1,
		ActivityType: "send-email",
		Input:        json.RawMessage(`{}`),
		// TaskQueue intentionally empty -> should fallback to workflow's queue
	})
	commands := []domain.Command{
		{Type: domain.CommandScheduleActivityTask, Attributes: attrs},
	}

	err := e.processCommands(context.Background(), wfID, "workflow-queue", commands)
	require.NoError(t, err)
	assert.Equal(t, "workflow-queue", enqueuedTask.QueueName)
	assert.Equal(t, "send-email", enqueuedTask.ActivityType)
	assert.Equal(t, 1, enqueuedTask.MaxAttempts)
}

func TestProcessCommands_ScheduleActivity_ExplicitTaskQueue(t *testing.T) {
	var enqueuedTask domain.ActivityTask

	e := newTestEngine(
		&mockWorkflowRepo{},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{
			EnqueueFn: func(_ context.Context, task domain.ActivityTask) error {
				enqueuedTask = task
				return nil
			},
		},
		&mockTimerRepo{},
		&mockQueryRepo{},
	)

	wfID := uuid.New()
	attrs, _ := json.Marshal(domain.ScheduleActivityTaskAttributes{
		SeqID:        1,
		ActivityType: "process-payment",
		TaskQueue:    "payment-queue",
	})
	commands := []domain.Command{
		{Type: domain.CommandScheduleActivityTask, Attributes: attrs},
	}

	err := e.processCommands(context.Background(), wfID, "workflow-queue", commands)
	require.NoError(t, err)
	assert.Equal(t, "payment-queue", enqueuedTask.QueueName)
}

func TestProcessCommands_ScheduleActivity_RetryPolicyPropagation(t *testing.T) {
	var enqueuedTask domain.ActivityTask

	e := newTestEngine(
		&mockWorkflowRepo{},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{
			EnqueueFn: func(_ context.Context, task domain.ActivityTask) error {
				enqueuedTask = task
				return nil
			},
		},
		&mockTimerRepo{},
		&mockQueryRepo{},
	)

	wfID := uuid.New()
	attrs, _ := json.Marshal(domain.ScheduleActivityTaskAttributes{
		SeqID:        1,
		ActivityType: "send-email",
		RetryPolicy: &domain.RetryPolicy{
			MaxAttempts: 5,
		},
	})
	commands := []domain.Command{
		{Type: domain.CommandScheduleActivityTask, Attributes: attrs},
	}

	err := e.processCommands(context.Background(), wfID, "default", commands)
	require.NoError(t, err)
	assert.Equal(t, 5, enqueuedTask.MaxAttempts)
	assert.NotNil(t, enqueuedTask.RetryPolicy)
}

func TestProcessCommands_CompleteWorkflow(t *testing.T) {
	var updatedStatus domain.WorkflowStatus
	var appendedEvents []domain.HistoryEvent
	wfID := uuid.New()

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusCompleted}, nil
			},
			UpdateStatusFn: func(_ context.Context, _ uuid.UUID, status domain.WorkflowStatus, _ json.RawMessage, _ string) error {
				updatedStatus = status
				return nil
			},
		},
		&mockEventRepo{
			AppendFn: func(_ context.Context, events []domain.HistoryEvent) error {
				appendedEvents = events
				return nil
			},
		},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
		&mockQueryRepo{},
	)

	attrs, _ := json.Marshal(domain.CompleteWorkflowAttributes{
		Result: json.RawMessage(`{"done":true}`),
	})
	commands := []domain.Command{
		{Type: domain.CommandCompleteWorkflow, Attributes: attrs},
	}

	err := e.processCommands(context.Background(), wfID, "default", commands)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkflowStatusCompleted, updatedStatus)
	require.Len(t, appendedEvents, 1)
	assert.Equal(t, domain.EventWorkflowExecutionCompleted, appendedEvents[0].Type)
}

func TestProcessCommands_FailWorkflow(t *testing.T) {
	var updatedStatus domain.WorkflowStatus
	var updatedErrMsg string
	wfID := uuid.New()

	e := newTestEngine(
		&mockWorkflowRepo{
			GetFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
				return &domain.WorkflowExecution{ID: wfID, TaskQueue: "default", Status: domain.WorkflowStatusFailed}, nil
			},
			UpdateStatusFn: func(_ context.Context, _ uuid.UUID, status domain.WorkflowStatus, _ json.RawMessage, errMsg string) error {
				updatedStatus = status
				updatedErrMsg = errMsg
				return nil
			},
		},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
		&mockQueryRepo{},
	)

	attrs, _ := json.Marshal(domain.FailWorkflowAttributes{
		ErrorMessage: "workflow failed",
	})
	commands := []domain.Command{
		{Type: domain.CommandFailWorkflow, Attributes: attrs},
	}

	err := e.processCommands(context.Background(), wfID, "default", commands)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkflowStatusFailed, updatedStatus)
	assert.Equal(t, "workflow failed", updatedErrMsg)
}

func TestProcessCommands_UnknownCommand(t *testing.T) {
	e := newTestEngine(
		&mockWorkflowRepo{},
		&mockEventRepo{},
		&mockWorkflowTaskRepo{},
		&mockActivityTaskRepo{},
		&mockTimerRepo{},
		&mockQueryRepo{},
	)

	commands := []domain.Command{
		{Type: "UnknownCommand", Attributes: json.RawMessage(`{}`)},
	}

	err := e.processCommands(context.Background(), uuid.New(), "default", commands)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command type")
}
