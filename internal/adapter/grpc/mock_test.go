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
	SignalWorkflowFn    func(ctx context.Context, id uuid.UUID, signalName string, input json.RawMessage) error
	CancelWorkflowFn   func(ctx context.Context, id uuid.UUID) error
	ListWorkflowsFn    func(ctx context.Context, params port.ListWorkflowsParams) (*port.ListWorkflowsResult, error)
	QueryWorkflowFn    func(ctx context.Context, id uuid.UUID, queryType string, input json.RawMessage) (*domain.WorkflowQuery, error)
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

func (m *mockClientService) SignalWorkflow(ctx context.Context, id uuid.UUID, signalName string, input json.RawMessage) error {
	if m.SignalWorkflowFn != nil {
		return m.SignalWorkflowFn(ctx, id, signalName, input)
	}
	return nil
}

func (m *mockClientService) CancelWorkflow(ctx context.Context, id uuid.UUID) error {
	if m.CancelWorkflowFn != nil {
		return m.CancelWorkflowFn(ctx, id)
	}
	return nil
}

func (m *mockClientService) ListWorkflows(ctx context.Context, params port.ListWorkflowsParams) (*port.ListWorkflowsResult, error) {
	if m.ListWorkflowsFn != nil {
		return m.ListWorkflowsFn(ctx, params)
	}
	return &port.ListWorkflowsResult{}, nil
}

func (m *mockClientService) QueryWorkflow(ctx context.Context, id uuid.UUID, queryType string, input json.RawMessage) (*domain.WorkflowQuery, error) {
	if m.QueryWorkflowFn != nil {
		return m.QueryWorkflowFn(ctx, id, queryType, input)
	}
	return nil, nil
}

// --- mockWorkflowTaskService ---

type mockWorkflowTaskService struct {
	PollWorkflowTaskFn     func(ctx context.Context, queueName string, workerID string) (*port.WorkflowTaskResult, error)
	CompleteWorkflowTaskFn func(ctx context.Context, taskID int64, commands []domain.Command) error
	FailWorkflowTaskFn     func(ctx context.Context, taskID int64, cause string, message string) error
	RespondQueryTaskFn     func(ctx context.Context, queryID int64, result json.RawMessage, errMsg string) error
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

func (m *mockWorkflowTaskService) RespondQueryTask(ctx context.Context, queryID int64, result json.RawMessage, errMsg string) error {
	if m.RespondQueryTaskFn != nil {
		return m.RespondQueryTaskFn(ctx, queryID, result, errMsg)
	}
	return nil
}

// --- mockActivityTaskService ---

type mockActivityTaskService struct {
	PollActivityTaskFn         func(ctx context.Context, queueName string, workerID string) (*domain.ActivityTask, error)
	CompleteActivityTaskFn     func(ctx context.Context, taskID int64, result json.RawMessage) error
	FailActivityTaskFn         func(ctx context.Context, taskID int64, failure domain.ActivityFailure) error
	RecordActivityHeartbeatFn  func(ctx context.Context, taskID int64, details json.RawMessage) error
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

func (m *mockActivityTaskService) RecordActivityHeartbeat(ctx context.Context, taskID int64, details json.RawMessage) error {
	if m.RecordActivityHeartbeatFn != nil {
		return m.RecordActivityHeartbeatFn(ctx, taskID, details)
	}
	return nil
}
