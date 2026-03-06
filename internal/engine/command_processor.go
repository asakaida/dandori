package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/google/uuid"
)

func (e *Engine) processCommands(ctx context.Context, workflowID uuid.UUID, taskQueue string, commands []domain.Command) error {
	for _, cmd := range commands {
		switch cmd.Type {
		case domain.CommandScheduleActivityTask:
			if err := e.processScheduleActivity(ctx, workflowID, taskQueue, cmd.Attributes, cmd.Metadata); err != nil {
				return err
			}
		case domain.CommandCompleteWorkflow:
			if err := e.processCompleteWorkflow(ctx, workflowID, cmd.Attributes, cmd.Metadata); err != nil {
				return err
			}
		case domain.CommandFailWorkflow:
			if err := e.processFailWorkflow(ctx, workflowID, cmd.Attributes, cmd.Metadata); err != nil {
				return err
			}
		case domain.CommandStartTimer:
			if err := e.processStartTimer(ctx, workflowID, cmd.Attributes, cmd.Metadata); err != nil {
				return err
			}
		case domain.CommandCancelTimer:
			if err := e.processCancelTimer(ctx, workflowID, cmd.Attributes, cmd.Metadata); err != nil {
				return err
			}
		case domain.CommandStartChildWorkflow:
			if err := e.processStartChildWorkflow(ctx, workflowID, taskQueue, cmd.Attributes, cmd.Metadata); err != nil {
				return err
			}
		case domain.CommandRecordSideEffect:
			if err := e.processRecordSideEffect(ctx, workflowID, cmd.Attributes, cmd.Metadata); err != nil {
				return err
			}
		case domain.CommandContinueAsNew:
			if err := e.processContinueAsNew(ctx, workflowID, cmd.Attributes, cmd.Metadata); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown command type: %s", cmd.Type)
		}
	}
	return nil
}

func (e *Engine) processScheduleActivity(ctx context.Context, workflowID uuid.UUID, taskQueue string, attrs json.RawMessage, metadata map[string]string) error {
	var a domain.ScheduleActivityTaskAttributes
	if err := json.Unmarshal(attrs, &a); err != nil {
		return fmt.Errorf("unmarshal ScheduleActivityTaskAttributes: %w", err)
	}

	queue := a.TaskQueue
	if queue == "" {
		queue = taskQueue
	}

	maxAttempts := 1
	if a.RetryPolicy != nil && a.RetryPolicy.MaxAttempts > 0 {
		maxAttempts = a.RetryPolicy.MaxAttempts
	}

	now := time.Now()
	task := domain.ActivityTask{
		QueueName:              queue,
		WorkflowID:             workflowID,
		ActivityType:           a.ActivityType,
		ActivityInput:          a.Input,
		ActivitySeqID:          a.SeqID,
		StartToCloseTimeout:    a.StartToCloseTimeout,
		HeartbeatTimeout:       a.HeartbeatTimeout,
		ScheduleToCloseTimeout: a.ScheduleToCloseTimeout,
		ScheduleToStartTimeout: a.ScheduleToStartTimeout,
		Attempt:                1,
		MaxAttempts:            maxAttempts,
		RetryPolicy:            a.RetryPolicy,
		ScheduledAt:            now,
	}
	if a.ScheduleToCloseTimeout > 0 {
		t := now.Add(a.ScheduleToCloseTimeout)
		task.ScheduleToCloseTimeoutAt = &t
	}
	if a.ScheduleToStartTimeout > 0 {
		t := now.Add(a.ScheduleToStartTimeout)
		task.ScheduleToStartTimeoutAt = &t
	}
	if err := e.activityTasks.Enqueue(ctx, task); err != nil {
		return err
	}

	eventData, err := marshalEventData(a, metadata)
	if err != nil {
		return err
	}
	return e.events.Append(ctx, []domain.HistoryEvent{
		{WorkflowID: workflowID, Type: domain.EventActivityTaskScheduled, Data: eventData},
	})
}

func (e *Engine) processCompleteWorkflow(ctx context.Context, workflowID uuid.UUID, attrs json.RawMessage, metadata map[string]string) error {
	var a domain.CompleteWorkflowAttributes
	if err := json.Unmarshal(attrs, &a); err != nil {
		return fmt.Errorf("unmarshal CompleteWorkflowAttributes: %w", err)
	}

	wf, err := e.workflows.Get(ctx, workflowID)
	if err != nil {
		return err
	}

	if wf.CronSchedule != "" {
		eventData, err := marshalEventData(a, metadata)
		if err != nil {
			return err
		}
		if err := e.events.Append(ctx, []domain.HistoryEvent{
			{WorkflowID: workflowID, Type: domain.EventWorkflowExecutionCompleted, Data: eventData},
		}); err != nil {
			return err
		}
		return e.continueAsNew(ctx, wf, wf.WorkflowType, wf.TaskQueue, a.Result, metadata)
	}

	if err := e.workflows.UpdateStatus(ctx, workflowID, domain.WorkflowStatusCompleted, a.Result, ""); err != nil {
		return err
	}

	eventData, err := marshalEventData(a, metadata)
	if err != nil {
		return err
	}
	if err := e.events.Append(ctx, []domain.HistoryEvent{
		{WorkflowID: workflowID, Type: domain.EventWorkflowExecutionCompleted, Data: eventData},
	}); err != nil {
		return err
	}

	return e.propagateToParent(ctx, wf, domain.EventChildWorkflowExecutionCompleted, map[string]any{
		"child_workflow_id": workflowID.String(),
		"seq_id":            wf.ParentSeqID,
		"result":            a.Result,
	})
}

func (e *Engine) processFailWorkflow(ctx context.Context, workflowID uuid.UUID, attrs json.RawMessage, metadata map[string]string) error {
	var a domain.FailWorkflowAttributes
	if err := json.Unmarshal(attrs, &a); err != nil {
		return fmt.Errorf("unmarshal FailWorkflowAttributes: %w", err)
	}

	if err := e.workflows.UpdateStatus(ctx, workflowID, domain.WorkflowStatusFailed, nil, a.ErrorMessage); err != nil {
		return err
	}

	eventData, err := marshalEventData(a, metadata)
	if err != nil {
		return err
	}
	if err := e.events.Append(ctx, []domain.HistoryEvent{
		{WorkflowID: workflowID, Type: domain.EventWorkflowExecutionFailed, Data: eventData},
	}); err != nil {
		return err
	}

	wf, err := e.workflows.Get(ctx, workflowID)
	if err != nil {
		return err
	}
	return e.propagateToParent(ctx, wf, domain.EventChildWorkflowExecutionFailed, map[string]any{
		"child_workflow_id": workflowID.String(),
		"seq_id":            wf.ParentSeqID,
		"error_message":     a.ErrorMessage,
	})
}

func (e *Engine) processStartTimer(ctx context.Context, workflowID uuid.UUID, attrs json.RawMessage, metadata map[string]string) error {
	var a domain.StartTimerAttributes
	if err := json.Unmarshal(attrs, &a); err != nil {
		return fmt.Errorf("unmarshal StartTimerAttributes: %w", err)
	}

	timer := domain.Timer{
		WorkflowID: workflowID,
		SeqID:      a.SeqID,
		FireAt:     time.Now().Add(a.Duration),
	}
	if err := e.timers.Create(ctx, timer); err != nil {
		return err
	}

	eventData, err := marshalEventData(a, metadata)
	if err != nil {
		return err
	}
	return e.events.Append(ctx, []domain.HistoryEvent{
		{WorkflowID: workflowID, Type: domain.EventTimerStarted, Data: eventData},
	})
}

func (e *Engine) processCancelTimer(ctx context.Context, workflowID uuid.UUID, attrs json.RawMessage, metadata map[string]string) error {
	var a domain.CancelTimerAttributes
	if err := json.Unmarshal(attrs, &a); err != nil {
		return fmt.Errorf("unmarshal CancelTimerAttributes: %w", err)
	}

	canceled, err := e.timers.Cancel(ctx, workflowID, a.SeqID)
	if err != nil {
		return err
	}

	if !canceled {
		return nil
	}

	eventData, err := marshalEventData(a, metadata)
	if err != nil {
		return err
	}
	return e.events.Append(ctx, []domain.HistoryEvent{
		{WorkflowID: workflowID, Type: domain.EventTimerCanceled, Data: eventData},
	})
}

func (e *Engine) processStartChildWorkflow(ctx context.Context, workflowID uuid.UUID, taskQueue string, attrs json.RawMessage, metadata map[string]string) error {
	var a domain.StartChildWorkflowAttributes
	if err := json.Unmarshal(attrs, &a); err != nil {
		return fmt.Errorf("unmarshal StartChildWorkflowAttributes: %w", err)
	}

	childID := uuid.New()
	if a.WorkflowID != "" {
		parsed, err := uuid.Parse(a.WorkflowID)
		if err != nil {
			return fmt.Errorf("parse child workflow_id: %w", err)
		}
		childID = parsed
	}

	childQueue := a.TaskQueue
	if childQueue == "" {
		childQueue = taskQueue
	}

	eventData, err := marshalEventData(map[string]any{
		"seq_id":             a.SeqID,
		"child_workflow_id":  childID.String(),
		"workflow_type":      a.WorkflowType,
		"task_queue":         childQueue,
	}, metadata)
	if err != nil {
		return err
	}
	if err := e.events.Append(ctx, []domain.HistoryEvent{
		{WorkflowID: workflowID, Type: domain.EventChildWorkflowExecutionStarted, Data: eventData},
	}); err != nil {
		return err
	}

	childWF := domain.WorkflowExecution{
		ID:               childID,
		WorkflowType:     a.WorkflowType,
		TaskQueue:        childQueue,
		Status:           domain.WorkflowStatusRunning,
		Input:            a.Input,
		ParentWorkflowID: &workflowID,
		ParentSeqID:      a.SeqID,
	}
	if err := e.workflows.Create(ctx, childWF); err != nil {
		return err
	}

	childEventData, err := json.Marshal(map[string]json.RawMessage{"input": a.Input})
	if err != nil {
		return err
	}
	if err := e.events.Append(ctx, []domain.HistoryEvent{
		{WorkflowID: childID, Type: domain.EventWorkflowExecutionStarted, Data: childEventData},
	}); err != nil {
		return err
	}

	return e.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
		QueueName:   childQueue,
		WorkflowID:  childID,
		ScheduledAt: time.Now(),
	})
}

func (e *Engine) processRecordSideEffect(ctx context.Context, workflowID uuid.UUID, attrs json.RawMessage, metadata map[string]string) error {
	var a domain.RecordSideEffectAttributes
	if err := json.Unmarshal(attrs, &a); err != nil {
		return fmt.Errorf("unmarshal RecordSideEffectAttributes: %w", err)
	}

	eventData, err := marshalEventData(a, metadata)
	if err != nil {
		return err
	}
	return e.events.Append(ctx, []domain.HistoryEvent{
		{WorkflowID: workflowID, Type: domain.EventSideEffectRecorded, Data: eventData},
	})
}

func (e *Engine) processContinueAsNew(ctx context.Context, workflowID uuid.UUID, attrs json.RawMessage, metadata map[string]string) error {
	var a domain.ContinueAsNewAttributes
	if err := json.Unmarshal(attrs, &a); err != nil {
		return fmt.Errorf("unmarshal ContinueAsNewAttributes: %w", err)
	}

	wf, err := e.workflows.Get(ctx, workflowID)
	if err != nil {
		return err
	}

	wfType := a.WorkflowType
	if wfType == "" {
		wfType = wf.WorkflowType
	}
	taskQueue := a.TaskQueue
	if taskQueue == "" {
		taskQueue = wf.TaskQueue
	}

	return e.continueAsNew(ctx, wf, wfType, taskQueue, a.Input, metadata)
}

func (e *Engine) continueAsNew(ctx context.Context, wf *domain.WorkflowExecution, workflowType string, taskQueue string, input json.RawMessage, metadata map[string]string) error {
	newID := uuid.New()

	if err := e.workflows.UpdateStatus(ctx, wf.ID, domain.WorkflowStatusContinuedAsNew, nil, ""); err != nil {
		return err
	}

	newWF := domain.WorkflowExecution{
		ID:           newID,
		WorkflowType: workflowType,
		TaskQueue:    taskQueue,
		Status:       domain.WorkflowStatusRunning,
		Input:        input,
		CronSchedule: wf.CronSchedule,
	}
	if err := e.workflows.Create(ctx, newWF); err != nil {
		return err
	}

	if err := e.workflows.SetContinuedAsNewID(ctx, wf.ID, newID); err != nil {
		return err
	}

	canEventData, err := marshalEventData(map[string]any{
		"new_workflow_id": newID.String(),
		"workflow_type":   workflowType,
		"task_queue":      taskQueue,
	}, metadata)
	if err != nil {
		return err
	}
	if err := e.events.Append(ctx, []domain.HistoryEvent{
		{WorkflowID: wf.ID, Type: domain.EventWorkflowExecutionContinuedAsNew, Data: canEventData},
	}); err != nil {
		return err
	}

	startEventData, err := json.Marshal(map[string]json.RawMessage{"input": input})
	if err != nil {
		return err
	}
	if err := e.events.Append(ctx, []domain.HistoryEvent{
		{WorkflowID: newID, Type: domain.EventWorkflowExecutionStarted, Data: startEventData},
	}); err != nil {
		return err
	}

	return e.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
		QueueName:   taskQueue,
		WorkflowID:  newID,
		ScheduledAt: time.Now(),
	})
}

func marshalEventData(attrs any, metadata map[string]string) (json.RawMessage, error) {
	if len(metadata) == 0 {
		return json.Marshal(attrs)
	}
	base, err := json.Marshal(attrs)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(base, &m); err != nil {
		return nil, err
	}
	mdJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	m["metadata"] = mdJSON
	return json.Marshal(m)
}
