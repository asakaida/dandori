package telemetry

import (
	"context"
	"encoding/json"
	"time"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/asakaida/dandori/internal/port"
	"github.com/google/uuid"
)

// --- MetricsClientService ---

type MetricsClientService struct {
	next    port.ClientService
	metrics *Metrics
}

func NewMetricsClientService(next port.ClientService, metrics *Metrics) *MetricsClientService {
	return &MetricsClientService{next: next, metrics: metrics}
}

func (s *MetricsClientService) StartWorkflow(ctx context.Context, params port.StartWorkflowParams) (*domain.WorkflowExecution, error) {
	start := time.Now()
	wf, err := s.next.StartWorkflow(ctx, params)
	s.metrics.OperationDuration.WithLabelValues("StartWorkflow").Observe(time.Since(start).Seconds())
	if err == nil {
		s.metrics.WorkflowStartedTotal.Inc()
		s.metrics.ActiveWorkflows.Inc()
	}
	return wf, err
}

func (s *MetricsClientService) DescribeWorkflow(ctx context.Context, namespace string, id uuid.UUID) (*domain.WorkflowExecution, error) {
	start := time.Now()
	wf, err := s.next.DescribeWorkflow(ctx, namespace, id)
	s.metrics.OperationDuration.WithLabelValues("DescribeWorkflow").Observe(time.Since(start).Seconds())
	return wf, err
}

func (s *MetricsClientService) GetWorkflowHistory(ctx context.Context, namespace string, workflowID uuid.UUID) ([]domain.HistoryEvent, error) {
	start := time.Now()
	events, err := s.next.GetWorkflowHistory(ctx, namespace, workflowID)
	s.metrics.OperationDuration.WithLabelValues("GetWorkflowHistory").Observe(time.Since(start).Seconds())
	return events, err
}

func (s *MetricsClientService) TerminateWorkflow(ctx context.Context, namespace string, id uuid.UUID, reason string) error {
	start := time.Now()
	err := s.next.TerminateWorkflow(ctx, namespace, id, reason)
	s.metrics.OperationDuration.WithLabelValues("TerminateWorkflow").Observe(time.Since(start).Seconds())
	if err == nil {
		s.metrics.WorkflowTerminatedTotal.Inc()
		s.metrics.ActiveWorkflows.Dec()
	}
	return err
}

func (s *MetricsClientService) SignalWorkflow(ctx context.Context, namespace string, id uuid.UUID, signalName string, input json.RawMessage) error {
	start := time.Now()
	err := s.next.SignalWorkflow(ctx, namespace, id, signalName, input)
	s.metrics.OperationDuration.WithLabelValues("SignalWorkflow").Observe(time.Since(start).Seconds())
	return err
}

func (s *MetricsClientService) CancelWorkflow(ctx context.Context, namespace string, id uuid.UUID) error {
	start := time.Now()
	err := s.next.CancelWorkflow(ctx, namespace, id)
	s.metrics.OperationDuration.WithLabelValues("CancelWorkflow").Observe(time.Since(start).Seconds())
	if err == nil {
		s.metrics.WorkflowCanceledTotal.Inc()
	}
	return err
}

func (s *MetricsClientService) ListWorkflows(ctx context.Context, params port.ListWorkflowsParams) (*port.ListWorkflowsResult, error) {
	start := time.Now()
	result, err := s.next.ListWorkflows(ctx, params)
	s.metrics.OperationDuration.WithLabelValues("ListWorkflows").Observe(time.Since(start).Seconds())
	return result, err
}

func (s *MetricsClientService) QueryWorkflow(ctx context.Context, namespace string, id uuid.UUID, queryType string, input json.RawMessage) (*domain.WorkflowQuery, error) {
	start := time.Now()
	q, err := s.next.QueryWorkflow(ctx, namespace, id, queryType, input)
	s.metrics.OperationDuration.WithLabelValues("QueryWorkflow").Observe(time.Since(start).Seconds())
	return q, err
}

// --- MetricsWorkflowTaskService ---

type MetricsWorkflowTaskService struct {
	next    port.WorkflowTaskService
	metrics *Metrics
}

func NewMetricsWorkflowTaskService(next port.WorkflowTaskService, metrics *Metrics) *MetricsWorkflowTaskService {
	return &MetricsWorkflowTaskService{next: next, metrics: metrics}
}

func (s *MetricsWorkflowTaskService) PollWorkflowTask(ctx context.Context, namespace string, queueName string, workerID string) (*port.WorkflowTaskResult, error) {
	start := time.Now()
	result, err := s.next.PollWorkflowTask(ctx, namespace, queueName, workerID)
	s.metrics.OperationDuration.WithLabelValues("PollWorkflowTask").Observe(time.Since(start).Seconds())
	s.metrics.WorkflowTaskPollTotal.Inc()
	return result, err
}

func (s *MetricsWorkflowTaskService) CompleteWorkflowTask(ctx context.Context, taskID int64, commands []domain.Command) error {
	start := time.Now()
	err := s.next.CompleteWorkflowTask(ctx, taskID, commands)
	s.metrics.OperationDuration.WithLabelValues("CompleteWorkflowTask").Observe(time.Since(start).Seconds())
	if err == nil {
		s.metrics.WorkflowTaskCompleteTotal.Inc()
	}
	return err
}

func (s *MetricsWorkflowTaskService) FailWorkflowTask(ctx context.Context, taskID int64, cause string, message string) error {
	start := time.Now()
	err := s.next.FailWorkflowTask(ctx, taskID, cause, message)
	s.metrics.OperationDuration.WithLabelValues("FailWorkflowTask").Observe(time.Since(start).Seconds())
	if err == nil {
		s.metrics.WorkflowTaskFailTotal.Inc()
		s.metrics.ActiveWorkflows.Dec()
	}
	return err
}

func (s *MetricsWorkflowTaskService) RespondQueryTask(ctx context.Context, queryID int64, result json.RawMessage, errMsg string) error {
	start := time.Now()
	err := s.next.RespondQueryTask(ctx, queryID, result, errMsg)
	s.metrics.OperationDuration.WithLabelValues("RespondQueryTask").Observe(time.Since(start).Seconds())
	return err
}

// --- MetricsActivityTaskService ---

type MetricsActivityTaskService struct {
	next    port.ActivityTaskService
	metrics *Metrics
}

func NewMetricsActivityTaskService(next port.ActivityTaskService, metrics *Metrics) *MetricsActivityTaskService {
	return &MetricsActivityTaskService{next: next, metrics: metrics}
}

func (s *MetricsActivityTaskService) PollActivityTask(ctx context.Context, namespace string, queueName string, workerID string) (*domain.ActivityTask, error) {
	start := time.Now()
	task, err := s.next.PollActivityTask(ctx, namespace, queueName, workerID)
	s.metrics.OperationDuration.WithLabelValues("PollActivityTask").Observe(time.Since(start).Seconds())
	s.metrics.ActivityTaskPollTotal.Inc()
	return task, err
}

func (s *MetricsActivityTaskService) CompleteActivityTask(ctx context.Context, taskID int64, result json.RawMessage) error {
	start := time.Now()
	err := s.next.CompleteActivityTask(ctx, taskID, result)
	s.metrics.OperationDuration.WithLabelValues("CompleteActivityTask").Observe(time.Since(start).Seconds())
	if err == nil {
		s.metrics.ActivityTaskCompleteTotal.Inc()
	}
	return err
}

func (s *MetricsActivityTaskService) FailActivityTask(ctx context.Context, taskID int64, failure domain.ActivityFailure) error {
	start := time.Now()
	err := s.next.FailActivityTask(ctx, taskID, failure)
	s.metrics.OperationDuration.WithLabelValues("FailActivityTask").Observe(time.Since(start).Seconds())
	if err == nil {
		s.metrics.ActivityTaskFailTotal.Inc()
	}
	return err
}

func (s *MetricsActivityTaskService) RecordActivityHeartbeat(ctx context.Context, taskID int64, details json.RawMessage) error {
	start := time.Now()
	err := s.next.RecordActivityHeartbeat(ctx, taskID, details)
	s.metrics.OperationDuration.WithLabelValues("RecordActivityHeartbeat").Observe(time.Since(start).Seconds())
	return err
}
