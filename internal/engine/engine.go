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
	queries       port.QueryRepository
	namespaces    port.NamespaceRepository
	tx            port.TxManager
	queryTimeout  time.Duration
	broadcaster   *Broadcaster
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
	queries port.QueryRepository,
	namespaces port.NamespaceRepository,
	tx port.TxManager,
	broadcaster *Broadcaster,
) *Engine {
	return &Engine{
		workflows:     workflows,
		events:        events,
		workflowTasks: workflowTasks,
		activityTasks: activityTasks,
		timers:        timers,
		queries:       queries,
		namespaces:    namespaces,
		tx:            tx,
		queryTimeout:  10 * time.Second,
		broadcaster:   broadcaster,
	}
}

func resolveNamespace(ns string) string {
	if ns == "" {
		return "default"
	}
	return ns
}

// --- ClientService ---

func (e *Engine) StartWorkflow(ctx context.Context, params port.StartWorkflowParams) (*domain.WorkflowExecution, error) {
	params.Namespace = resolveNamespace(params.Namespace)

	if params.ID == uuid.Nil {
		params.ID = uuid.New()
	}

	if params.CronSchedule != "" {
		if err := ValidateCronSchedule(params.CronSchedule); err != nil {
			return nil, err
		}
	}

	var wf *domain.WorkflowExecution
	err := e.tx.RunInTx(ctx, func(ctx context.Context) error {
		existing, err := e.workflows.Get(ctx, params.Namespace, params.ID)
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
			if err := e.queries.DeleteByWorkflowID(ctx, params.ID); err != nil {
				return err
			}
		}

		newWF := domain.WorkflowExecution{
			ID:           params.ID,
			Namespace:    params.Namespace,
			WorkflowType: params.WorkflowType,
			TaskQueue:    params.TaskQueue,
			Status:       domain.WorkflowStatusRunning,
			Input:        params.Input,
			CronSchedule: params.CronSchedule,
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
			Namespace:   params.Namespace,
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

	e.broadcaster.Publish(WorkflowNotification{
		WorkflowID: wf.ID.String(),
		Namespace:  wf.Namespace,
	})

	return wf, nil
}

func (e *Engine) DescribeWorkflow(ctx context.Context, namespace string, id uuid.UUID) (*domain.WorkflowExecution, error) {
	namespace = resolveNamespace(namespace)
	return e.workflows.Get(ctx, namespace, id)
}

func (e *Engine) GetWorkflowHistory(ctx context.Context, namespace string, workflowID uuid.UUID) ([]domain.HistoryEvent, error) {
	namespace = resolveNamespace(namespace)
	if _, err := e.workflows.Get(ctx, namespace, workflowID); err != nil {
		return nil, err
	}
	return e.events.GetByWorkflowID(ctx, workflowID)
}

func (e *Engine) TerminateWorkflow(ctx context.Context, namespace string, id uuid.UUID, reason string) error {
	namespace = resolveNamespace(namespace)
	err := e.tx.RunInTx(ctx, func(ctx context.Context) error {
		wf, err := e.workflows.Get(ctx, namespace, id)
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
	if err == nil {
		e.broadcaster.Publish(WorkflowNotification{
			WorkflowID: id.String(),
			Namespace:  namespace,
		})
	}
	return err
}

func (e *Engine) SignalWorkflow(ctx context.Context, namespace string, id uuid.UUID, signalName string, input json.RawMessage) error {
	namespace = resolveNamespace(namespace)
	return e.tx.RunInTx(ctx, func(ctx context.Context) error {
		wf, err := e.workflows.Get(ctx, namespace, id)
		if err != nil {
			return err
		}
		if wf.Status != domain.WorkflowStatusRunning {
			return domain.ErrWorkflowNotRunning
		}

		eventData, err := json.Marshal(map[string]any{
			"signal_name": signalName,
			"input":       input,
		})
		if err != nil {
			return err
		}
		if err := e.events.Append(ctx, []domain.HistoryEvent{
			{WorkflowID: id, Type: domain.EventWorkflowSignaled, Data: eventData},
		}); err != nil {
			return err
		}

		return e.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
			Namespace:   wf.Namespace,
			QueueName:   wf.TaskQueue,
			WorkflowID:  id,
			ScheduledAt: time.Now(),
		})
	})
}

func (e *Engine) ListWorkflows(ctx context.Context, params port.ListWorkflowsParams) (*port.ListWorkflowsResult, error) {
	params.Namespace = resolveNamespace(params.Namespace)
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}

	params.PageSize++ // fetch one extra to detect next page
	workflows, err := e.workflows.List(ctx, params)
	if err != nil {
		return nil, err
	}
	params.PageSize-- // restore original

	var nextCursor *port.ListWorkflowsCursor
	if len(workflows) > params.PageSize {
		last := workflows[params.PageSize-1]
		nextCursor = &port.ListWorkflowsCursor{
			CreatedAt: last.CreatedAt,
			ID:        last.ID,
		}
		workflows = workflows[:params.PageSize]
	}

	return &port.ListWorkflowsResult{
		Workflows:  workflows,
		NextCursor: nextCursor,
	}, nil
}

func (e *Engine) CancelWorkflow(ctx context.Context, namespace string, id uuid.UUID) error {
	namespace = resolveNamespace(namespace)
	return e.tx.RunInTx(ctx, func(ctx context.Context) error {
		wf, err := e.workflows.Get(ctx, namespace, id)
		if err != nil {
			return err
		}
		if wf.Status != domain.WorkflowStatusRunning {
			return domain.ErrWorkflowNotRunning
		}

		if err := e.events.Append(ctx, []domain.HistoryEvent{
			{WorkflowID: id, Type: domain.EventWorkflowCancelRequested, Data: json.RawMessage(`{}`)},
		}); err != nil {
			return err
		}

		return e.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
			Namespace:   wf.Namespace,
			QueueName:   wf.TaskQueue,
			WorkflowID:  id,
			ScheduledAt: time.Now(),
		})
	})
}

// --- WorkflowTaskService ---

func (e *Engine) PollWorkflowTask(ctx context.Context, namespace string, queueName string, workerID string) (*port.WorkflowTaskResult, error) {
	namespace = resolveNamespace(namespace)
	task, err := e.workflowTasks.Poll(ctx, namespace, queueName, workerID)
	if errors.Is(err, domain.ErrNoTaskAvailable) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	wf, err := e.workflows.Get(ctx, task.Namespace, task.WorkflowID)
	if err != nil {
		return nil, err
	}

	events, err := e.events.GetByWorkflowID(ctx, task.WorkflowID)
	if err != nil {
		return nil, err
	}

	pendingQueries, err := e.queries.GetPendingByWorkflowID(ctx, task.WorkflowID)
	if err != nil {
		return nil, err
	}

	return &port.WorkflowTaskResult{
		Task:           *task,
		Events:         events,
		WorkflowType:   wf.WorkflowType,
		PendingQueries: pendingQueries,
	}, nil
}

func (e *Engine) CompleteWorkflowTask(ctx context.Context, taskID int64, commands []domain.Command) error {
	var wfID uuid.UUID
	var ns string
	err := e.tx.RunInTx(ctx, func(ctx context.Context) error {
		task, err := e.workflowTasks.GetByID(ctx, taskID)
		if err != nil {
			return err
		}
		wfID = task.WorkflowID
		ns = task.Namespace

		if err := e.workflowTasks.Complete(ctx, taskID); err != nil {
			return err
		}

		wf, err := e.workflows.Get(ctx, task.Namespace, task.WorkflowID)
		if err != nil {
			return err
		}

		return e.processCommands(ctx, wf, commands)
	})
	if err == nil {
		e.broadcaster.Publish(WorkflowNotification{
			WorkflowID: wfID.String(),
			Namespace:  ns,
		})
	}
	return err
}

func (e *Engine) FailWorkflowTask(ctx context.Context, taskID int64, cause string, message string) error {
	var wfID uuid.UUID
	var ns string
	err := e.tx.RunInTx(ctx, func(ctx context.Context) error {
		task, err := e.workflowTasks.GetByID(ctx, taskID)
		if err != nil {
			return err
		}
		wfID = task.WorkflowID
		ns = task.Namespace

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
		if err := e.events.Append(ctx, []domain.HistoryEvent{
			{WorkflowID: task.WorkflowID, Type: domain.EventWorkflowExecutionFailed, Data: eventData},
		}); err != nil {
			return err
		}

		wf, err := e.workflows.Get(ctx, task.Namespace, task.WorkflowID)
		if err != nil {
			return err
		}
		return e.propagateToParent(ctx, wf, domain.EventChildWorkflowExecutionFailed, map[string]any{
			"child_workflow_id": task.WorkflowID.String(),
			"seq_id":            wf.ParentSeqID,
			"error_message":     message,
		})
	})
	if err == nil {
		e.broadcaster.Publish(WorkflowNotification{
			WorkflowID: wfID.String(),
			Namespace:  ns,
		})
	}
	return err
}

func (e *Engine) RespondQueryTask(ctx context.Context, queryID int64, result json.RawMessage, errMsg string) error {
	return e.queries.SetResult(ctx, queryID, result, errMsg)
}

func (e *Engine) QueryWorkflow(ctx context.Context, namespace string, id uuid.UUID, queryType string, input json.RawMessage) (*domain.WorkflowQuery, error) {
	namespace = resolveNamespace(namespace)
	var queryID int64
	err := e.tx.RunInTx(ctx, func(ctx context.Context) error {
		wf, err := e.workflows.Get(ctx, namespace, id)
		if err != nil {
			return err
		}
		if wf.Status != domain.WorkflowStatusRunning {
			return domain.ErrWorkflowNotRunning
		}

		queryID, err = e.queries.Create(ctx, domain.WorkflowQuery{
			WorkflowID: id,
			QueryType:  queryType,
			Input:      input,
			Status:     domain.QueryStatusPending,
		})
		if err != nil {
			return err
		}

		return e.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
			Namespace:   wf.Namespace,
			QueueName:   wf.TaskQueue,
			WorkflowID:  id,
			ScheduledAt: time.Now(),
		})
	})
	if err != nil {
		return nil, err
	}

	deadline := time.After(e.queryTimeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, domain.ErrQueryTimedOut
		case <-ticker.C:
			q, err := e.queries.GetByID(ctx, queryID)
			if err != nil {
				return nil, err
			}
			if q.Status == domain.QueryStatusAnswered {
				return q, nil
			}
		}
	}
}

// --- ActivityTaskService ---

func (e *Engine) PollActivityTask(ctx context.Context, namespace string, queueName string, workerID string) (*domain.ActivityTask, error) {
	namespace = resolveNamespace(namespace)
	task, err := e.activityTasks.Poll(ctx, namespace, queueName, workerID)
	if errors.Is(err, domain.ErrNoTaskAvailable) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (e *Engine) RecordActivityHeartbeat(ctx context.Context, taskID int64, details json.RawMessage) error {
	return e.activityTasks.UpdateHeartbeat(ctx, taskID)
}

func (e *Engine) CompleteActivityTask(ctx context.Context, taskID int64, result json.RawMessage) error {
	return e.tx.RunInTx(ctx, func(ctx context.Context) error {
		task, err := e.activityTasks.GetByID(ctx, taskID)
		if err != nil {
			return err
		}

		wf, err := e.workflows.Get(ctx, task.Namespace, task.WorkflowID)
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
			Namespace:   wf.Namespace,
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

		wf, err := e.workflows.Get(ctx, task.Namespace, task.WorkflowID)
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
				Namespace:   wf.Namespace,
				QueueName:   wf.TaskQueue,
				WorkflowID:  task.WorkflowID,
				ScheduledAt: time.Now(),
			})
		}

		return e.activityTasks.Requeue(ctx, taskID, computeNextRetryTime(task))
	})
}

func (e *Engine) propagateToParent(ctx context.Context, childWF *domain.WorkflowExecution, eventType domain.EventType, data map[string]any) error {
	if childWF.ParentWorkflowID == nil {
		return nil
	}

	parentWF, err := e.workflows.Get(ctx, childWF.Namespace, *childWF.ParentWorkflowID)
	if err != nil {
		return err
	}
	if parentWF.Status != domain.WorkflowStatusRunning {
		return nil
	}

	eventData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if err := e.events.Append(ctx, []domain.HistoryEvent{
		{WorkflowID: parentWF.ID, Type: eventType, Data: eventData},
	}); err != nil {
		return err
	}

	return e.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
		Namespace:   parentWF.Namespace,
		QueueName:   parentWF.TaskQueue,
		WorkflowID:  parentWF.ID,
		ScheduledAt: time.Now(),
	})
}
