package port

import (
	"context"
	"encoding/json"
	"time"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/google/uuid"
)

type WorkflowRepository interface {
	Create(ctx context.Context, wf domain.WorkflowExecution) error
	Get(ctx context.Context, id uuid.UUID) (*domain.WorkflowExecution, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.WorkflowStatus, result json.RawMessage, errMsg string) error
}

type EventRepository interface {
	Append(ctx context.Context, events []domain.HistoryEvent) error
	GetByWorkflowID(ctx context.Context, workflowID uuid.UUID) ([]domain.HistoryEvent, error)
	DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error
}

type WorkflowTaskRepository interface {
	Enqueue(ctx context.Context, task domain.WorkflowTask) error
	Poll(ctx context.Context, queueName string, workerID string) (*domain.WorkflowTask, error)
	Complete(ctx context.Context, taskID int64) error
	GetByID(ctx context.Context, taskID int64) (*domain.WorkflowTask, error)
	RecoverStaleTasks(ctx context.Context) (int, error)
	DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error
}

type ActivityTaskRepository interface {
	Enqueue(ctx context.Context, task domain.ActivityTask) error
	Poll(ctx context.Context, queueName string, workerID string) (*domain.ActivityTask, error)
	Complete(ctx context.Context, taskID int64) error
	GetByID(ctx context.Context, taskID int64) (*domain.ActivityTask, error)
	GetTimedOut(ctx context.Context) ([]domain.ActivityTask, error)
	Requeue(ctx context.Context, taskID int64, scheduledAt time.Time) error
	RecoverStaleTasks(ctx context.Context) (int, error)
	DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error
}

type TimerRepository interface {
	Create(ctx context.Context, timer domain.Timer) error
	GetFired(ctx context.Context) ([]domain.Timer, error)
	MarkFired(ctx context.Context, timerID int64) error
	DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error
}

type TxManager interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}
