package telemetry

import (
	"context"
	"encoding/json"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/asakaida/dandori/internal/port"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// --- TracingClientService ---

type TracingClientService struct {
	next   port.ClientService
	tracer trace.Tracer
}

func NewTracingClientService(next port.ClientService, tracer trace.Tracer) *TracingClientService {
	return &TracingClientService{next: next, tracer: tracer}
}

func (s *TracingClientService) StartWorkflow(ctx context.Context, params port.StartWorkflowParams) (*domain.WorkflowExecution, error) {
	ctx, span := s.tracer.Start(ctx, "ClientService.StartWorkflow",
		trace.WithAttributes(
			attribute.String("namespace", params.Namespace),
			attribute.String("workflow.id", params.ID.String()),
			attribute.String("workflow.type", params.WorkflowType),
			attribute.String("workflow.task_queue", params.TaskQueue),
		),
	)
	defer span.End()
	wf, err := s.next.StartWorkflow(ctx, params)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return wf, err
}

func (s *TracingClientService) DescribeWorkflow(ctx context.Context, namespace string, id uuid.UUID) (*domain.WorkflowExecution, error) {
	ctx, span := s.tracer.Start(ctx, "ClientService.DescribeWorkflow",
		trace.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("workflow.id", id.String()),
		),
	)
	defer span.End()
	wf, err := s.next.DescribeWorkflow(ctx, namespace, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return wf, err
}

func (s *TracingClientService) GetWorkflowHistory(ctx context.Context, namespace string, workflowID uuid.UUID) ([]domain.HistoryEvent, error) {
	ctx, span := s.tracer.Start(ctx, "ClientService.GetWorkflowHistory",
		trace.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("workflow.id", workflowID.String()),
		),
	)
	defer span.End()
	events, err := s.next.GetWorkflowHistory(ctx, namespace, workflowID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return events, err
}

func (s *TracingClientService) TerminateWorkflow(ctx context.Context, namespace string, id uuid.UUID, reason string) error {
	ctx, span := s.tracer.Start(ctx, "ClientService.TerminateWorkflow",
		trace.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("workflow.id", id.String()),
		),
	)
	defer span.End()
	err := s.next.TerminateWorkflow(ctx, namespace, id, reason)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (s *TracingClientService) SignalWorkflow(ctx context.Context, namespace string, id uuid.UUID, signalName string, input json.RawMessage) error {
	ctx, span := s.tracer.Start(ctx, "ClientService.SignalWorkflow",
		trace.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("workflow.id", id.String()),
			attribute.String("signal.name", signalName),
		),
	)
	defer span.End()
	err := s.next.SignalWorkflow(ctx, namespace, id, signalName, input)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (s *TracingClientService) CancelWorkflow(ctx context.Context, namespace string, id uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "ClientService.CancelWorkflow",
		trace.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("workflow.id", id.String()),
		),
	)
	defer span.End()
	err := s.next.CancelWorkflow(ctx, namespace, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (s *TracingClientService) ListWorkflows(ctx context.Context, params port.ListWorkflowsParams) (*port.ListWorkflowsResult, error) {
	ctx, span := s.tracer.Start(ctx, "ClientService.ListWorkflows",
		trace.WithAttributes(
			attribute.String("namespace", params.Namespace),
			attribute.Int("list.page_size", params.PageSize),
			attribute.String("list.status_filter", params.StatusFilter),
		),
	)
	defer span.End()
	result, err := s.next.ListWorkflows(ctx, params)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

func (s *TracingClientService) QueryWorkflow(ctx context.Context, namespace string, id uuid.UUID, queryType string, input json.RawMessage) (*domain.WorkflowQuery, error) {
	ctx, span := s.tracer.Start(ctx, "ClientService.QueryWorkflow",
		trace.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("workflow.id", id.String()),
			attribute.String("query.type", queryType),
		),
	)
	defer span.End()
	q, err := s.next.QueryWorkflow(ctx, namespace, id, queryType, input)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return q, err
}

// --- TracingWorkflowTaskService ---

type TracingWorkflowTaskService struct {
	next   port.WorkflowTaskService
	tracer trace.Tracer
}

func NewTracingWorkflowTaskService(next port.WorkflowTaskService, tracer trace.Tracer) *TracingWorkflowTaskService {
	return &TracingWorkflowTaskService{next: next, tracer: tracer}
}

func (s *TracingWorkflowTaskService) PollWorkflowTask(ctx context.Context, namespace string, queueName string, workerID string) (*port.WorkflowTaskResult, error) {
	ctx, span := s.tracer.Start(ctx, "WorkflowTaskService.PollWorkflowTask",
		trace.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("task.queue", queueName),
			attribute.String("worker.id", workerID),
		),
	)
	defer span.End()
	result, err := s.next.PollWorkflowTask(ctx, namespace, queueName, workerID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

func (s *TracingWorkflowTaskService) CompleteWorkflowTask(ctx context.Context, taskID int64, commands []domain.Command) error {
	ctx, span := s.tracer.Start(ctx, "WorkflowTaskService.CompleteWorkflowTask",
		trace.WithAttributes(
			attribute.Int64("task.id", taskID),
			attribute.Int("commands.count", len(commands)),
		),
	)
	defer span.End()
	err := s.next.CompleteWorkflowTask(ctx, taskID, commands)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (s *TracingWorkflowTaskService) FailWorkflowTask(ctx context.Context, taskID int64, cause string, message string) error {
	ctx, span := s.tracer.Start(ctx, "WorkflowTaskService.FailWorkflowTask",
		trace.WithAttributes(attribute.Int64("task.id", taskID)),
	)
	defer span.End()
	err := s.next.FailWorkflowTask(ctx, taskID, cause, message)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (s *TracingWorkflowTaskService) RespondQueryTask(ctx context.Context, queryID int64, result json.RawMessage, errMsg string) error {
	ctx, span := s.tracer.Start(ctx, "WorkflowTaskService.RespondQueryTask",
		trace.WithAttributes(attribute.Int64("query.id", queryID)),
	)
	defer span.End()
	err := s.next.RespondQueryTask(ctx, queryID, result, errMsg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

// --- TracingActivityTaskService ---

type TracingActivityTaskService struct {
	next   port.ActivityTaskService
	tracer trace.Tracer
}

func NewTracingActivityTaskService(next port.ActivityTaskService, tracer trace.Tracer) *TracingActivityTaskService {
	return &TracingActivityTaskService{next: next, tracer: tracer}
}

func (s *TracingActivityTaskService) PollActivityTask(ctx context.Context, namespace string, queueName string, workerID string) (*domain.ActivityTask, error) {
	ctx, span := s.tracer.Start(ctx, "ActivityTaskService.PollActivityTask",
		trace.WithAttributes(
			attribute.String("namespace", namespace),
			attribute.String("task.queue", queueName),
			attribute.String("worker.id", workerID),
		),
	)
	defer span.End()
	task, err := s.next.PollActivityTask(ctx, namespace, queueName, workerID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return task, err
}

func (s *TracingActivityTaskService) CompleteActivityTask(ctx context.Context, taskID int64, result json.RawMessage) error {
	ctx, span := s.tracer.Start(ctx, "ActivityTaskService.CompleteActivityTask",
		trace.WithAttributes(attribute.Int64("task.id", taskID)),
	)
	defer span.End()
	err := s.next.CompleteActivityTask(ctx, taskID, result)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (s *TracingActivityTaskService) FailActivityTask(ctx context.Context, taskID int64, failure domain.ActivityFailure) error {
	ctx, span := s.tracer.Start(ctx, "ActivityTaskService.FailActivityTask",
		trace.WithAttributes(attribute.Int64("task.id", taskID)),
	)
	defer span.End()
	err := s.next.FailActivityTask(ctx, taskID, failure)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (s *TracingActivityTaskService) RecordActivityHeartbeat(ctx context.Context, taskID int64, details json.RawMessage) error {
	ctx, span := s.tracer.Start(ctx, "ActivityTaskService.RecordActivityHeartbeat",
		trace.WithAttributes(attribute.Int64("task.id", taskID)),
	)
	defer span.End()
	err := s.next.RecordActivityHeartbeat(ctx, taskID, details)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}
