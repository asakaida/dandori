package telemetry_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/asakaida/dandori/internal/adapter/telemetry"
	"github.com/asakaida/dandori/internal/domain"
	"github.com/asakaida/dandori/internal/port"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func setupTracer() (*tracetest.InMemoryExporter, *sdktrace.TracerProvider) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	return exporter, tp
}

// --- mock services ---

type mockClientService struct {
	startWorkflowFn func(ctx context.Context, params port.StartWorkflowParams) (*domain.WorkflowExecution, error)
}

func (m *mockClientService) StartWorkflow(ctx context.Context, params port.StartWorkflowParams) (*domain.WorkflowExecution, error) {
	if m.startWorkflowFn != nil {
		return m.startWorkflowFn(ctx, params)
	}
	return &domain.WorkflowExecution{ID: params.ID}, nil
}
func (m *mockClientService) DescribeWorkflow(_ context.Context, _ string, id uuid.UUID) (*domain.WorkflowExecution, error) {
	return &domain.WorkflowExecution{ID: id}, nil
}
func (m *mockClientService) GetWorkflowHistory(context.Context, string, uuid.UUID) ([]domain.HistoryEvent, error) {
	return nil, nil
}
func (m *mockClientService) TerminateWorkflow(context.Context, string, uuid.UUID, string) error {
	return nil
}
func (m *mockClientService) SignalWorkflow(context.Context, string, uuid.UUID, string, json.RawMessage) error {
	return nil
}
func (m *mockClientService) CancelWorkflow(context.Context, string, uuid.UUID) error { return nil }
func (m *mockClientService) ListWorkflows(context.Context, port.ListWorkflowsParams) (*port.ListWorkflowsResult, error) {
	return &port.ListWorkflowsResult{}, nil
}
func (m *mockClientService) QueryWorkflow(context.Context, string, uuid.UUID, string, json.RawMessage) (*domain.WorkflowQuery, error) {
	return &domain.WorkflowQuery{}, nil
}

type mockWorkflowTaskService struct{}

func (m *mockWorkflowTaskService) PollWorkflowTask(context.Context, string, string, string) (*port.WorkflowTaskResult, error) {
	return nil, nil
}
func (m *mockWorkflowTaskService) CompleteWorkflowTask(context.Context, int64, []domain.Command) error {
	return nil
}
func (m *mockWorkflowTaskService) FailWorkflowTask(context.Context, int64, string, string) error {
	return nil
}
func (m *mockWorkflowTaskService) RespondQueryTask(context.Context, int64, json.RawMessage, string) error {
	return nil
}

type mockActivityTaskService struct{}

func (m *mockActivityTaskService) PollActivityTask(context.Context, string, string, string) (*domain.ActivityTask, error) {
	return nil, nil
}
func (m *mockActivityTaskService) CompleteActivityTask(context.Context, int64, json.RawMessage) error {
	return nil
}
func (m *mockActivityTaskService) FailActivityTask(context.Context, int64, domain.ActivityFailure) error {
	return nil
}
func (m *mockActivityTaskService) RecordActivityHeartbeat(context.Context, int64, json.RawMessage) error {
	return nil
}

// --- tests ---

func TestTracingClientService_StartWorkflow_CreatesSpan(t *testing.T) {
	exporter, tp := setupTracer()
	defer tp.Shutdown(context.Background())
	tracer := tp.Tracer("test")

	mock := &mockClientService{}
	svc := telemetry.NewTracingClientService(mock, tracer)

	wfID := uuid.New()
	wf, err := svc.StartWorkflow(context.Background(), port.StartWorkflowParams{
		ID:           wfID,
		WorkflowType: "TestWorkflow",
		TaskQueue:    "default",
	})
	require.NoError(t, err)
	assert.Equal(t, wfID, wf.ID)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "ClientService.StartWorkflow", spans[0].Name)
}

func TestTracingClientService_StartWorkflow_RecordsError(t *testing.T) {
	exporter, tp := setupTracer()
	defer tp.Shutdown(context.Background())
	tracer := tp.Tracer("test")

	mock := &mockClientService{
		startWorkflowFn: func(ctx context.Context, params port.StartWorkflowParams) (*domain.WorkflowExecution, error) {
			return nil, domain.ErrWorkflowAlreadyExists
		},
	}
	svc := telemetry.NewTracingClientService(mock, tracer)

	_, err := svc.StartWorkflow(context.Background(), port.StartWorkflowParams{
		ID:           uuid.New(),
		WorkflowType: "TestWorkflow",
		TaskQueue:    "default",
	})
	assert.Error(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "ClientService.StartWorkflow", spans[0].Name)
	assert.NotEmpty(t, spans[0].Events)
}

func TestTracingWorkflowTaskService_PollCreatesSpan(t *testing.T) {
	exporter, tp := setupTracer()
	defer tp.Shutdown(context.Background())
	tracer := tp.Tracer("test")

	mock := &mockWorkflowTaskService{}
	svc := telemetry.NewTracingWorkflowTaskService(mock, tracer)

	_, err := svc.PollWorkflowTask(context.Background(), "default", "default", "worker-1")
	require.NoError(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "WorkflowTaskService.PollWorkflowTask", spans[0].Name)
}

func TestTracingActivityTaskService_PollCreatesSpan(t *testing.T) {
	exporter, tp := setupTracer()
	defer tp.Shutdown(context.Background())
	tracer := tp.Tracer("test")

	mock := &mockActivityTaskService{}
	svc := telemetry.NewTracingActivityTaskService(mock, tracer)

	_, err := svc.PollActivityTask(context.Background(), "default", "default", "worker-1")
	require.NoError(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "ActivityTaskService.PollActivityTask", spans[0].Name)
}
