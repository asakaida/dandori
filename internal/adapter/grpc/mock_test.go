package grpc_test

import (
	"context"
	"encoding/json"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/asakaida/dandori/internal/port"
	"github.com/google/uuid"
)

// --- mockClientService ---

type mockClientService struct {
	StartWorkflowFn     func(ctx context.Context, params port.StartWorkflowParams) (*domain.WorkflowExecution, error)
	DescribeWorkflowFn  func(ctx context.Context, id uuid.UUID) (*domain.WorkflowExecution, error)
	GetWorkflowHistoryFn func(ctx context.Context, workflowID uuid.UUID) ([]domain.HistoryEvent, error)
	TerminateWorkflowFn func(ctx context.Context, id uuid.UUID, reason string) error
}

func (m *mockClientService) StartWorkflow(ctx context.Context, params port.StartWorkflowParams) (*domain.WorkflowExecution, error) {
	if m.StartWorkflowFn != nil {
		return m.StartWorkflowFn(ctx, params)
	}
	return nil, nil
}

func (m *mockClientService) DescribeWorkflow(ctx context.Context, id uuid.UUID) (*domain.WorkflowExecution, error) {
	if m.DescribeWorkflowFn != nil {
		return m.DescribeWorkflowFn(ctx, id)
	}
	return nil, domain.ErrWorkflowNotFound
}

func (m *mockClientService) GetWorkflowHistory(ctx context.Context, workflowID uuid.UUID) ([]domain.HistoryEvent, error) {
	if m.GetWorkflowHistoryFn != nil {
		return m.GetWorkflowHistoryFn(ctx, workflowID)
	}
	return nil, nil
}

func (m *mockClientService) TerminateWorkflow(ctx context.Context, id uuid.UUID, reason string) error {
	if m.TerminateWorkflowFn != nil {
		return m.TerminateWorkflowFn(ctx, id, reason)
	}
	return nil
}

// --- mockWorkflowTaskService ---

type mockWorkflowTaskService struct {
	PollWorkflowTaskFn     func(ctx context.Context, queueName string, workerID string) (*port.WorkflowTaskResult, error)
	CompleteWorkflowTaskFn func(ctx context.Context, taskID int64, commands []domain.Command) error
	FailWorkflowTaskFn     func(ctx context.Context, taskID int64, cause string, message string) error
}

func (m *mockWorkflowTaskService) PollWorkflowTask(ctx context.Context, queueName string, workerID string) (*port.WorkflowTaskResult, error) {
	if m.PollWorkflowTaskFn != nil {
		return m.PollWorkflowTaskFn(ctx, queueName, workerID)
	}
	return nil, nil
}

func (m *mockWorkflowTaskService) CompleteWorkflowTask(ctx context.Context, taskID int64, commands []domain.Command) error {
	if m.CompleteWorkflowTaskFn != nil {
		return m.CompleteWorkflowTaskFn(ctx, taskID, commands)
	}
	return nil
}

func (m *mockWorkflowTaskService) FailWorkflowTask(ctx context.Context, taskID int64, cause string, message string) error {
	if m.FailWorkflowTaskFn != nil {
		return m.FailWorkflowTaskFn(ctx, taskID, cause, message)
	}
	return nil
}

// --- mockActivityTaskService ---

type mockActivityTaskService struct {
	PollActivityTaskFn     func(ctx context.Context, queueName string, workerID string) (*domain.ActivityTask, error)
	CompleteActivityTaskFn func(ctx context.Context, taskID int64, result json.RawMessage) error
	FailActivityTaskFn     func(ctx context.Context, taskID int64, failure domain.ActivityFailure) error
}

func (m *mockActivityTaskService) PollActivityTask(ctx context.Context, queueName string, workerID string) (*domain.ActivityTask, error) {
	if m.PollActivityTaskFn != nil {
		return m.PollActivityTaskFn(ctx, queueName, workerID)
	}
	return nil, nil
}

func (m *mockActivityTaskService) CompleteActivityTask(ctx context.Context, taskID int64, result json.RawMessage) error {
	if m.CompleteActivityTaskFn != nil {
		return m.CompleteActivityTaskFn(ctx, taskID, result)
	}
	return nil
}

func (m *mockActivityTaskService) FailActivityTask(ctx context.Context, taskID int64, failure domain.ActivityFailure) error {
	if m.FailActivityTaskFn != nil {
		return m.FailActivityTaskFn(ctx, taskID, failure)
	}
	return nil
}
