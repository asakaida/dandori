package engine

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/asakaida/dandori/internal/port"
	"github.com/google/uuid"
)

type Engine struct {
	workflows     port.WorkflowRepository
	events        port.EventRepository
	workflowTasks port.WorkflowTaskRepository
	activityTasks port.ActivityTaskRepository
	timers        port.TimerRepository
	tx            port.TxManager
}

var _ port.ClientService = (*Engine)(nil)
var _ port.WorkflowTaskService = (*Engine)(nil)
var _ port.ActivityTaskService = (*Engine)(nil)

func New(
	workflows port.WorkflowRepository,
	events port.EventRepository,
	workflowTasks port.WorkflowTaskRepository,
	activityTasks port.ActivityTaskRepository,
	timers port.TimerRepository,
	tx port.TxManager,
) *Engine {
	return &Engine{
		workflows:     workflows,
		events:        events,
		workflowTasks: workflowTasks,
		activityTasks: activityTasks,
		timers:        timers,
		tx:            tx,
	}
}

// --- ClientService ---

func (e *Engine) StartWorkflow(ctx context.Context, params port.StartWorkflowParams) (*domain.WorkflowExecution, error) {
	if params.ID == uuid.Nil {
		params.ID = uuid.New()
	}

	var wf *domain.WorkflowExecution
	err := e.tx.RunInTx(ctx, func(ctx context.Context) error {
		existing, err := e.workflows.Get(ctx, params.ID)
		if err != nil && !errors.Is(err, domain.ErrWorkflowNotFound) {
			return err
		}

		if existing != nil && existing.Status == domain.WorkflowStatusRunning {
			return domain.ErrWorkflowAlreadyExists
		}

		if existing != nil && existing.Status.IsTerminal() {
			if err := e.events.DeleteByWorkflowID(ctx, params.ID); err != nil {
				return err
			}
			if err := e.workflowTasks.DeleteByWorkflowID(ctx, params.ID); err != nil {
				return err
			}
			if err := e.activityTasks.DeleteByWorkflowID(ctx, params.ID); err != nil {
				return err
			}
			if err := e.timers.DeleteByWorkflowID(ctx, params.ID); err != nil {
				return err
			}
		}

		newWF := domain.WorkflowExecution{
			ID:           params.ID,
			WorkflowType: params.WorkflowType,
			TaskQueue:    params.TaskQueue,
			Status:       domain.WorkflowStatusRunning,
			Input:        params.Input,
		}
		if err := e.workflows.Create(ctx, newWF); err != nil {
			return err
		}

		eventData, err := json.Marshal(map[string]json.RawMessage{"input": params.Input})
		if err != nil {
			return err
		}
		if err := e.events.Append(ctx, []domain.HistoryEvent{
			{WorkflowID: params.ID, Type: domain.EventWorkflowExecutionStarted, Data: eventData},
		}); err != nil {
			return err
		}

		if err := e.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
			QueueName:   params.TaskQueue,
			WorkflowID:  params.ID,
			ScheduledAt: time.Now(),
		}); err != nil {
			return err
		}

		wf = &newWF
		return nil
	})
	if err != nil {
		return nil, err
	}
	return wf, nil
}

func (e *Engine) DescribeWorkflow(ctx context.Context, id uuid.UUID) (*domain.WorkflowExecution, error) {
	return e.workflows.Get(ctx, id)
}

func (e *Engine) GetWorkflowHistory(ctx context.Context, workflowID uuid.UUID) ([]domain.HistoryEvent, error) {
	return e.events.GetByWorkflowID(ctx, workflowID)
}

func (e *Engine) TerminateWorkflow(ctx context.Context, id uuid.UUID, reason string) error {
	return e.tx.RunInTx(ctx, func(ctx context.Context) error {
		wf, err := e.workflows.Get(ctx, id)
		if err != nil {
			return err
		}
		if wf.Status != domain.WorkflowStatusRunning {
			return domain.ErrWorkflowNotRunning
		}

		if err := e.workflows.UpdateStatus(ctx, id, domain.WorkflowStatusTerminated, nil, reason); err != nil {
			return err
		}

		eventData, err := json.Marshal(map[string]string{"reason": reason})
		if err != nil {
			return err
		}
		return e.events.Append(ctx, []domain.HistoryEvent{
			{WorkflowID: id, Type: domain.EventWorkflowExecutionTerminated, Data: eventData},
		})
	})
}

// --- WorkflowTaskService ---

func (e *Engine) PollWorkflowTask(ctx context.Context, queueName string, workerID string) (*port.WorkflowTaskResult, error) {
	task, err := e.workflowTasks.Poll(ctx, queueName, workerID)
	if errors.Is(err, domain.ErrNoTaskAvailable) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	wf, err := e.workflows.Get(ctx, task.WorkflowID)
	if err != nil {
		return nil, err
	}

	events, err := e.events.GetByWorkflowID(ctx, task.WorkflowID)
	if err != nil {
		return nil, err
	}

	return &port.WorkflowTaskResult{
		Task:         *task,
		Events:       events,
		WorkflowType: wf.WorkflowType,
	}, nil
}

func (e *Engine) CompleteWorkflowTask(ctx context.Context, taskID int64, commands []domain.Command) error {
	return e.tx.RunInTx(ctx, func(ctx context.Context) error {
		task, err := e.workflowTasks.GetByID(ctx, taskID)
		if err != nil {
			return err
		}

		if err := e.workflowTasks.Complete(ctx, taskID); err != nil {
			return err
		}

		wf, err := e.workflows.Get(ctx, task.WorkflowID)
		if err != nil {
			return err
		}

		return e.processCommands(ctx, task.WorkflowID, wf.TaskQueue, commands)
	})
}

func (e *Engine) FailWorkflowTask(ctx context.Context, taskID int64, cause string, message string) error {
	return e.tx.RunInTx(ctx, func(ctx context.Context) error {
		task, err := e.workflowTasks.GetByID(ctx, taskID)
		if err != nil {
			return err
		}

		if err := e.workflowTasks.Complete(ctx, taskID); err != nil {
			return err
		}

		if err := e.workflows.UpdateStatus(ctx, task.WorkflowID, domain.WorkflowStatusFailed, nil, message); err != nil {
			return err
		}

		eventData, err := json.Marshal(map[string]string{"cause": cause, "message": message})
		if err != nil {
			return err
		}
		return e.events.Append(ctx, []domain.HistoryEvent{
			{WorkflowID: task.WorkflowID, Type: domain.EventWorkflowExecutionFailed, Data: eventData},
		})
	})
}

// --- ActivityTaskService ---

func (e *Engine) PollActivityTask(ctx context.Context, queueName string, workerID string) (*domain.ActivityTask, error) {
	task, err := e.activityTasks.Poll(ctx, queueName, workerID)
	if errors.Is(err, domain.ErrNoTaskAvailable) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (e *Engine) CompleteActivityTask(ctx context.Context, taskID int64, result json.RawMessage) error {
	return e.tx.RunInTx(ctx, func(ctx context.Context) error {
		task, err := e.activityTasks.GetByID(ctx, taskID)
		if err != nil {
			return err
		}

		wf, err := e.workflows.Get(ctx, task.WorkflowID)
		if err != nil {
			return err
		}

		if err := e.activityTasks.Complete(ctx, taskID); err != nil {
			return err
		}

		if wf.Status.IsTerminal() {
			return nil
		}

		eventData, err := json.Marshal(map[string]any{
			"activity_seq_id": task.ActivitySeqID,
			"result":          result,
		})
		if err != nil {
			return err
		}
		if err := e.events.Append(ctx, []domain.HistoryEvent{
			{WorkflowID: task.WorkflowID, Type: domain.EventActivityTaskCompleted, Data: eventData},
		}); err != nil {
			return err
		}

		return e.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
			QueueName:   wf.TaskQueue,
			WorkflowID:  task.WorkflowID,
			ScheduledAt: time.Now(),
		})
	})
}

func (e *Engine) FailActivityTask(ctx context.Context, taskID int64, failure domain.ActivityFailure) error {
	return e.tx.RunInTx(ctx, func(ctx context.Context) error {
		task, err := e.activityTasks.GetByID(ctx, taskID)
		if err != nil {
			return err
		}

		wf, err := e.workflows.Get(ctx, task.WorkflowID)
		if err != nil {
			return err
		}

		if wf.Status.IsTerminal() {
			return e.activityTasks.Complete(ctx, taskID)
		}

		if failure.NonRetryable || task.Attempt >= task.MaxAttempts {
			if err := e.activityTasks.Complete(ctx, taskID); err != nil {
				return err
			}

			eventData, err := json.Marshal(map[string]any{
				"activity_seq_id": task.ActivitySeqID,
				"failure":         failure,
			})
			if err != nil {
				return err
			}
			if err := e.events.Append(ctx, []domain.HistoryEvent{
				{WorkflowID: task.WorkflowID, Type: domain.EventActivityTaskFailed, Data: eventData},
			}); err != nil {
				return err
			}

			return e.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
				QueueName:   wf.TaskQueue,
				WorkflowID:  task.WorkflowID,
				ScheduledAt: time.Now(),
			})
		}

		return e.activityTasks.Requeue(ctx, taskID, computeNextRetryTime(task))
	})
}
