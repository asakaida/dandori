package engine

import (
	"context"
	"encoding/json"
	"time"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/google/uuid"
)

type mockTxManager struct{}

func (m *mockTxManager) RunInTx(_ context.Context, fn func(ctx context.Context) error) error {
	return fn(context.Background())
}

type mockWorkflowRepo struct {
	CreateFn       func(ctx context.Context, wf domain.WorkflowExecution) error
	GetFn          func(ctx context.Context, id uuid.UUID) (*domain.WorkflowExecution, error)
	UpdateStatusFn func(ctx context.Context, id uuid.UUID, status domain.WorkflowStatus, result json.RawMessage, errMsg string) error
}

func (m *mockWorkflowRepo) Create(ctx context.Context, wf domain.WorkflowExecution) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, wf)
	}
	return nil
}

func (m *mockWorkflowRepo) Get(ctx context.Context, id uuid.UUID) (*domain.WorkflowExecution, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, domain.ErrWorkflowNotFound
}

func (m *mockWorkflowRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.WorkflowStatus, result json.RawMessage, errMsg string) error {
	if m.UpdateStatusFn != nil {
		return m.UpdateStatusFn(ctx, id, status, result, errMsg)
	}
	return nil
}

type mockEventRepo struct {
	AppendFn             func(ctx context.Context, events []domain.HistoryEvent) error
	GetByWorkflowIDFn    func(ctx context.Context, workflowID uuid.UUID) ([]domain.HistoryEvent, error)
	DeleteByWorkflowIDFn func(ctx context.Context, workflowID uuid.UUID) error
}

func (m *mockEventRepo) Append(ctx context.Context, events []domain.HistoryEvent) error {
	if m.AppendFn != nil {
		return m.AppendFn(ctx, events)
	}
	return nil
}

func (m *mockEventRepo) GetByWorkflowID(ctx context.Context, workflowID uuid.UUID) ([]domain.HistoryEvent, error) {
	if m.GetByWorkflowIDFn != nil {
		return m.GetByWorkflowIDFn(ctx, workflowID)
	}
	return nil, nil
}

func (m *mockEventRepo) DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error {
	if m.DeleteByWorkflowIDFn != nil {
		return m.DeleteByWorkflowIDFn(ctx, workflowID)
	}
	return nil
}

type mockWorkflowTaskRepo struct {
	EnqueueFn            func(ctx context.Context, task domain.WorkflowTask) error
	PollFn               func(ctx context.Context, queueName string, workerID string) (*domain.WorkflowTask, error)
	CompleteFn           func(ctx context.Context, taskID int64) error
	GetByIDFn            func(ctx context.Context, taskID int64) (*domain.WorkflowTask, error)
	RecoverStaleTasksFn  func(ctx context.Context) (int, error)
	DeleteByWorkflowIDFn func(ctx context.Context, workflowID uuid.UUID) error
}

func (m *mockWorkflowTaskRepo) Enqueue(ctx context.Context, task domain.WorkflowTask) error {
	if m.EnqueueFn != nil {
		return m.EnqueueFn(ctx, task)
	}
	return nil
}

func (m *mockWorkflowTaskRepo) Poll(ctx context.Context, queueName string, workerID string) (*domain.WorkflowTask, error) {
	if m.PollFn != nil {
		return m.PollFn(ctx, queueName, workerID)
	}
	return nil, domain.ErrNoTaskAvailable
}

func (m *mockWorkflowTaskRepo) Complete(ctx context.Context, taskID int64) error {
	if m.CompleteFn != nil {
		return m.CompleteFn(ctx, taskID)
	}
	return nil
}

func (m *mockWorkflowTaskRepo) GetByID(ctx context.Context, taskID int64) (*domain.WorkflowTask, error) {
	if m.GetByIDFn != nil {
		return m.GetByIDFn(ctx, taskID)
	}
	return nil, domain.ErrTaskNotFound
}

func (m *mockWorkflowTaskRepo) RecoverStaleTasks(ctx context.Context) (int, error) {
	if m.RecoverStaleTasksFn != nil {
		return m.RecoverStaleTasksFn(ctx)
	}
	return 0, nil
}

func (m *mockWorkflowTaskRepo) DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error {
	if m.DeleteByWorkflowIDFn != nil {
		return m.DeleteByWorkflowIDFn(ctx, workflowID)
	}
	return nil
}

type mockActivityTaskRepo struct {
	EnqueueFn               func(ctx context.Context, task domain.ActivityTask) error
	PollFn                  func(ctx context.Context, queueName string, workerID string) (*domain.ActivityTask, error)
	CompleteFn              func(ctx context.Context, taskID int64) error
	GetByIDFn               func(ctx context.Context, taskID int64) (*domain.ActivityTask, error)
	GetTimedOutFn           func(ctx context.Context) ([]domain.ActivityTask, error)
	RequeueFn               func(ctx context.Context, taskID int64, scheduledAt time.Time) error
	RecoverStaleTasksFn     func(ctx context.Context) (int, error)
	DeleteByWorkflowIDFn    func(ctx context.Context, workflowID uuid.UUID) error
	UpdateHeartbeatFn       func(ctx context.Context, taskID int64) error
	GetHeartbeatTimedOutFn  func(ctx context.Context) ([]domain.ActivityTask, error)
}

func (m *mockActivityTaskRepo) Enqueue(ctx context.Context, task domain.ActivityTask) error {
	if m.EnqueueFn != nil {
		return m.EnqueueFn(ctx, task)
	}
	return nil
}

func (m *mockActivityTaskRepo) Poll(ctx context.Context, queueName string, workerID string) (*domain.ActivityTask, error) {
	if m.PollFn != nil {
		return m.PollFn(ctx, queueName, workerID)
	}
	return nil, domain.ErrNoTaskAvailable
}

func (m *mockActivityTaskRepo) Complete(ctx context.Context, taskID int64) error {
	if m.CompleteFn != nil {
		return m.CompleteFn(ctx, taskID)
	}
	return nil
}

func (m *mockActivityTaskRepo) GetByID(ctx context.Context, taskID int64) (*domain.ActivityTask, error) {
	if m.GetByIDFn != nil {
		return m.GetByIDFn(ctx, taskID)
	}
	return nil, domain.ErrTaskNotFound
}

func (m *mockActivityTaskRepo) GetTimedOut(ctx context.Context) ([]domain.ActivityTask, error) {
	if m.GetTimedOutFn != nil {
		return m.GetTimedOutFn(ctx)
	}
	return nil, nil
}

func (m *mockActivityTaskRepo) Requeue(ctx context.Context, taskID int64, scheduledAt time.Time) error {
	if m.RequeueFn != nil {
		return m.RequeueFn(ctx, taskID, scheduledAt)
	}
	return nil
}

func (m *mockActivityTaskRepo) RecoverStaleTasks(ctx context.Context) (int, error) {
	if m.RecoverStaleTasksFn != nil {
		return m.RecoverStaleTasksFn(ctx)
	}
	return 0, nil
}

func (m *mockActivityTaskRepo) DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error {
	if m.DeleteByWorkflowIDFn != nil {
		return m.DeleteByWorkflowIDFn(ctx, workflowID)
	}
	return nil
}

func (m *mockActivityTaskRepo) UpdateHeartbeat(ctx context.Context, taskID int64) error {
	if m.UpdateHeartbeatFn != nil {
		return m.UpdateHeartbeatFn(ctx, taskID)
	}
	return nil
}

func (m *mockActivityTaskRepo) GetHeartbeatTimedOut(ctx context.Context) ([]domain.ActivityTask, error) {
	if m.GetHeartbeatTimedOutFn != nil {
		return m.GetHeartbeatTimedOutFn(ctx)
	}
	return nil, nil
}

type mockTimerRepo struct {
	CreateFn             func(ctx context.Context, timer domain.Timer) error
	GetFiredFn           func(ctx context.Context) ([]domain.Timer, error)
	MarkFiredFn          func(ctx context.Context, timerID int64) (bool, error)
	CancelFn             func(ctx context.Context, workflowID uuid.UUID, seqID int64) (bool, error)
	DeleteByWorkflowIDFn func(ctx context.Context, workflowID uuid.UUID) error
}

func (m *mockTimerRepo) Create(ctx context.Context, timer domain.Timer) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, timer)
	}
	return nil
}

func (m *mockTimerRepo) GetFired(ctx context.Context) ([]domain.Timer, error) {
	if m.GetFiredFn != nil {
		return m.GetFiredFn(ctx)
	}
	return nil, nil
}

func (m *mockTimerRepo) MarkFired(ctx context.Context, timerID int64) (bool, error) {
	if m.MarkFiredFn != nil {
		return m.MarkFiredFn(ctx, timerID)
	}
	return true, nil
}

func (m *mockTimerRepo) Cancel(ctx context.Context, workflowID uuid.UUID, seqID int64) (bool, error) {
	if m.CancelFn != nil {
		return m.CancelFn(ctx, workflowID, seqID)
	}
	return true, nil
}

func (m *mockTimerRepo) DeleteByWorkflowID(ctx context.Context, workflowID uuid.UUID) error {
	if m.DeleteByWorkflowIDFn != nil {
		return m.DeleteByWorkflowIDFn(ctx, workflowID)
	}
	return nil
}

func newTestEngine(
	wf *mockWorkflowRepo,
	ev *mockEventRepo,
	wt *mockWorkflowTaskRepo,
	at *mockActivityTaskRepo,
	tm *mockTimerRepo,
) *Engine {
	return New(wf, ev, wt, at, tm, &mockTxManager{})
}
