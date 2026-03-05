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
			if err := e.processScheduleActivity(ctx, workflowID, taskQueue, cmd.Attributes); err != nil {
				return err
			}
		case domain.CommandCompleteWorkflow:
			if err := e.processCompleteWorkflow(ctx, workflowID, cmd.Attributes); err != nil {
				return err
			}
		case domain.CommandFailWorkflow:
			if err := e.processFailWorkflow(ctx, workflowID, cmd.Attributes); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown command type: %s", cmd.Type)
		}
	}
	return nil
}

func (e *Engine) processScheduleActivity(ctx context.Context, workflowID uuid.UUID, taskQueue string, attrs json.RawMessage) error {
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

	task := domain.ActivityTask{
		QueueName:           queue,
		WorkflowID:          workflowID,
		ActivityType:        a.ActivityType,
		ActivityInput:       a.Input,
		ActivitySeqID:       a.SeqID,
		StartToCloseTimeout: a.StartToCloseTimeout,
		Attempt:             1,
		MaxAttempts:         maxAttempts,
		RetryPolicy:         a.RetryPolicy,
		ScheduledAt:         time.Now(),
	}
	if err := e.activityTasks.Enqueue(ctx, task); err != nil {
		return err
	}

	eventData, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return e.events.Append(ctx, []domain.HistoryEvent{
		{WorkflowID: workflowID, Type: domain.EventActivityTaskScheduled, Data: eventData},
	})
}

func (e *Engine) processCompleteWorkflow(ctx context.Context, workflowID uuid.UUID, attrs json.RawMessage) error {
	var a domain.CompleteWorkflowAttributes
	if err := json.Unmarshal(attrs, &a); err != nil {
		return fmt.Errorf("unmarshal CompleteWorkflowAttributes: %w", err)
	}

	if err := e.workflows.UpdateStatus(ctx, workflowID, domain.WorkflowStatusCompleted, a.Result, ""); err != nil {
		return err
	}

	eventData, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return e.events.Append(ctx, []domain.HistoryEvent{
		{WorkflowID: workflowID, Type: domain.EventWorkflowExecutionCompleted, Data: eventData},
	})
}

func (e *Engine) processFailWorkflow(ctx context.Context, workflowID uuid.UUID, attrs json.RawMessage) error {
	var a domain.FailWorkflowAttributes
	if err := json.Unmarshal(attrs, &a); err != nil {
		return fmt.Errorf("unmarshal FailWorkflowAttributes: %w", err)
	}

	if err := e.workflows.UpdateStatus(ctx, workflowID, domain.WorkflowStatusFailed, nil, a.ErrorMessage); err != nil {
		return err
	}

	eventData, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return e.events.Append(ctx, []domain.HistoryEvent{
		{WorkflowID: workflowID, Type: domain.EventWorkflowExecutionFailed, Data: eventData},
	})
}
