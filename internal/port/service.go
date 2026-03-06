package port

import (
	"context"
	"encoding/json"
	"time"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/google/uuid"
)

type ClientService interface {
	StartWorkflow(ctx context.Context, params StartWorkflowParams) (*domain.WorkflowExecution, error)
	DescribeWorkflow(ctx context.Context, namespace string, id uuid.UUID) (*domain.WorkflowExecution, error)
	GetWorkflowHistory(ctx context.Context, namespace string, workflowID uuid.UUID) ([]domain.HistoryEvent, error)
	TerminateWorkflow(ctx context.Context, namespace string, id uuid.UUID, reason string) error
	SignalWorkflow(ctx context.Context, namespace string, id uuid.UUID, signalName string, input json.RawMessage) error
	CancelWorkflow(ctx context.Context, namespace string, id uuid.UUID) error
	ListWorkflows(ctx context.Context, params ListWorkflowsParams) (*ListWorkflowsResult, error)
	QueryWorkflow(ctx context.Context, namespace string, id uuid.UUID, queryType string, input json.RawMessage) (*domain.WorkflowQuery, error)
}

type WorkflowTaskService interface {
	PollWorkflowTask(ctx context.Context, namespace string, queueName string, workerID string) (*WorkflowTaskResult, error)
	CompleteWorkflowTask(ctx context.Context, taskID int64, commands []domain.Command) error
	FailWorkflowTask(ctx context.Context, taskID int64, cause string, message string) error
	RespondQueryTask(ctx context.Context, queryID int64, result json.RawMessage, errMsg string) error
}

type ActivityTaskService interface {
	PollActivityTask(ctx context.Context, namespace string, queueName string, workerID string) (*domain.ActivityTask, error)
	CompleteActivityTask(ctx context.Context, taskID int64, result json.RawMessage) error
	FailActivityTask(ctx context.Context, taskID int64, failure domain.ActivityFailure) error
	RecordActivityHeartbeat(ctx context.Context, taskID int64, details json.RawMessage) error
}

type StartWorkflowParams struct {
	ID           uuid.UUID
	Namespace    string
	WorkflowType string
	TaskQueue    string
	Input        json.RawMessage
	CronSchedule string
}

type WorkflowTaskResult struct {
	Task           domain.WorkflowTask
	Events         []domain.HistoryEvent
	WorkflowType   string
	PendingQueries []domain.WorkflowQuery
}

type ListWorkflowsParams struct {
	Namespace    string
	PageSize     int
	Cursor       *ListWorkflowsCursor
	StatusFilter string
	TypeFilter   string
	QueueFilter  string
}

type ListWorkflowsCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

type ListWorkflowsResult struct {
	Workflows  []domain.WorkflowExecution
	NextCursor *ListWorkflowsCursor
}
