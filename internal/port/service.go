package port

import (
	"context"
	"encoding/json"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/google/uuid"
)

type ClientService interface {
	StartWorkflow(ctx context.Context, params StartWorkflowParams) (*domain.WorkflowExecution, error)
	DescribeWorkflow(ctx context.Context, id uuid.UUID) (*domain.WorkflowExecution, error)
	GetWorkflowHistory(ctx context.Context, workflowID uuid.UUID) ([]domain.HistoryEvent, error)
	TerminateWorkflow(ctx context.Context, id uuid.UUID, reason string) error
	SignalWorkflow(ctx context.Context, id uuid.UUID, signalName string, input json.RawMessage) error
	CancelWorkflow(ctx context.Context, id uuid.UUID) error
}

type WorkflowTaskService interface {
	PollWorkflowTask(ctx context.Context, queueName string, workerID string) (*WorkflowTaskResult, error)
	CompleteWorkflowTask(ctx context.Context, taskID int64, commands []domain.Command) error
	FailWorkflowTask(ctx context.Context, taskID int64, cause string, message string) error
}

type ActivityTaskService interface {
	PollActivityTask(ctx context.Context, queueName string, workerID string) (*domain.ActivityTask, error)
	CompleteActivityTask(ctx context.Context, taskID int64, result json.RawMessage) error
	FailActivityTask(ctx context.Context, taskID int64, failure domain.ActivityFailure) error
	RecordActivityHeartbeat(ctx context.Context, taskID int64, details json.RawMessage) error
}

type StartWorkflowParams struct {
	ID           uuid.UUID
	WorkflowType string
	TaskQueue    string
	Input        json.RawMessage
}

type WorkflowTaskResult struct {
	Task         domain.WorkflowTask
	Events       []domain.HistoryEvent
	WorkflowType string
}
