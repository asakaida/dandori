package grpc_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	apiv1 "github.com/asakaida/dandori/api/v1"
	adaptgrpc "github.com/asakaida/dandori/internal/adapter/grpc"
	"github.com/asakaida/dandori/internal/domain"
	"github.com/asakaida/dandori/internal/port"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name     string
		invoke   func(h *adaptgrpc.Handler) error
		wantCode codes.Code
	}{
		{
			name: "ErrWorkflowNotFound→NotFound",
			invoke: func(h *adaptgrpc.Handler) error {
				_, err := h.DescribeWorkflow(context.Background(), &apiv1.DescribeWorkflowRequest{
					WorkflowId: uuid.New().String(),
				})
				return err
			},
			wantCode: codes.NotFound,
		},
		{
			name: "ErrWorkflowAlreadyExists→AlreadyExists",
			invoke: func(h *adaptgrpc.Handler) error {
				_, err := h.StartWorkflow(context.Background(), &apiv1.StartWorkflowRequest{
					WorkflowId:   uuid.New().String(),
					WorkflowType: "test",
					TaskQueue:    "q",
				})
				return err
			},
			wantCode: codes.AlreadyExists,
		},
		{
			name: "ErrWorkflowNotRunning→FailedPrecondition",
			invoke: func(h *adaptgrpc.Handler) error {
				_, err := h.TerminateWorkflow(context.Background(), &apiv1.TerminateWorkflowRequest{
					WorkflowId: uuid.New().String(),
					Reason:     "test",
				})
				return err
			},
			wantCode: codes.FailedPrecondition,
		},
		{
			name: "ErrTaskNotFound→NotFound",
			invoke: func(h *adaptgrpc.Handler) error {
				_, err := h.CompleteWorkflowTask(context.Background(), &apiv1.CompleteWorkflowTaskRequest{
					TaskId: 999,
				})
				return err
			},
			wantCode: codes.NotFound,
		},
		{
			name: "ErrTaskAlreadyCompleted→FailedPrecondition",
			invoke: func(h *adaptgrpc.Handler) error {
				_, err := h.CompleteActivityTask(context.Background(), &apiv1.CompleteActivityTaskRequest{
					TaskId: 1,
					Result: []byte(`{}`),
				})
				return err
			},
			wantCode: codes.FailedPrecondition,
		},
		{
			name: "unknown_error→Internal",
			invoke: func(h *adaptgrpc.Handler) error {
				_, err := h.DescribeWorkflow(context.Background(), &apiv1.DescribeWorkflowRequest{
					WorkflowId: uuid.New().String(),
				})
				return err
			},
			wantCode: codes.Internal,
		},
		{
			name: "wrapped_error→unwrapped",
			invoke: func(h *adaptgrpc.Handler) error {
				_, err := h.DescribeWorkflow(context.Background(), &apiv1.DescribeWorkflowRequest{
					WorkflowId: uuid.New().String(),
				})
				return err
			},
			wantCode: codes.NotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h *adaptgrpc.Handler

			switch tt.name {
			case "ErrWorkflowNotFound→NotFound":
				h = adaptgrpc.NewHandler(
					&mockClientService{DescribeWorkflowFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
						return nil, domain.ErrWorkflowNotFound
					}},
					&mockWorkflowTaskService{},
					&mockActivityTaskService{},
				)
			case "ErrWorkflowAlreadyExists→AlreadyExists":
				h = adaptgrpc.NewHandler(
					&mockClientService{StartWorkflowFn: func(_ context.Context, _ port.StartWorkflowParams) (*domain.WorkflowExecution, error) {
						return nil, domain.ErrWorkflowAlreadyExists
					}},
					&mockWorkflowTaskService{},
					&mockActivityTaskService{},
				)
			case "ErrWorkflowNotRunning→FailedPrecondition":
				h = adaptgrpc.NewHandler(
					&mockClientService{TerminateWorkflowFn: func(_ context.Context, _ uuid.UUID, _ string) error {
						return domain.ErrWorkflowNotRunning
					}},
					&mockWorkflowTaskService{},
					&mockActivityTaskService{},
				)
			case "ErrTaskNotFound→NotFound":
				h = adaptgrpc.NewHandler(
					&mockClientService{},
					&mockWorkflowTaskService{CompleteWorkflowTaskFn: func(_ context.Context, _ int64, _ []domain.Command) error {
						return domain.ErrTaskNotFound
					}},
					&mockActivityTaskService{},
				)
			case "ErrTaskAlreadyCompleted→FailedPrecondition":
				h = adaptgrpc.NewHandler(
					&mockClientService{},
					&mockWorkflowTaskService{},
					&mockActivityTaskService{CompleteActivityTaskFn: func(_ context.Context, _ int64, _ json.RawMessage) error {
						return domain.ErrTaskAlreadyCompleted
					}},
				)
			case "unknown_error→Internal":
				h = adaptgrpc.NewHandler(
					&mockClientService{DescribeWorkflowFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
						return nil, errors.New("something unexpected")
					}},
					&mockWorkflowTaskService{},
					&mockActivityTaskService{},
				)
			case "wrapped_error→unwrapped":
				h = adaptgrpc.NewHandler(
					&mockClientService{DescribeWorkflowFn: func(_ context.Context, _ uuid.UUID) (*domain.WorkflowExecution, error) {
						return nil, fmt.Errorf("store layer: %w", domain.ErrWorkflowNotFound)
					}},
					&mockWorkflowTaskService{},
					&mockActivityTaskService{},
				)
			}

			err := tt.invoke(h)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected gRPC status error, got %v", err)
			}
			if st.Code() != tt.wantCode {
				t.Errorf("got code %v, want %v", st.Code(), tt.wantCode)
			}
		})
	}
}

func TestStartWorkflow_InvalidUUID(t *testing.T) {
	h := adaptgrpc.NewHandler(&mockClientService{}, &mockWorkflowTaskService{}, &mockActivityTaskService{})
	_, err := h.StartWorkflow(context.Background(), &apiv1.StartWorkflowRequest{
		WorkflowId:   "not-a-uuid",
		WorkflowType: "test",
		TaskQueue:    "q",
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestPollWorkflowTask_NoTask(t *testing.T) {
	h := adaptgrpc.NewHandler(&mockClientService{}, &mockWorkflowTaskService{
		PollWorkflowTaskFn: func(_ context.Context, _ string, _ string) (*port.WorkflowTaskResult, error) {
			return nil, nil
		},
	}, &mockActivityTaskService{})

	resp, err := h.PollWorkflowTask(context.Background(), &apiv1.PollWorkflowTaskRequest{
		QueueName: "q",
		WorkerId:  "w1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetTaskId() != 0 {
		t.Errorf("expected empty response, got task_id=%d", resp.GetTaskId())
	}
}

func TestPollActivityTask_NoTask(t *testing.T) {
	h := adaptgrpc.NewHandler(&mockClientService{}, &mockWorkflowTaskService{}, &mockActivityTaskService{
		PollActivityTaskFn: func(_ context.Context, _ string, _ string) (*domain.ActivityTask, error) {
			return nil, nil
		},
	})

	resp, err := h.PollActivityTask(context.Background(), &apiv1.PollActivityTaskRequest{
		QueueName: "q",
		WorkerId:  "w1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetTaskId() != 0 {
		t.Errorf("expected empty response, got task_id=%d", resp.GetTaskId())
	}
}

func TestSignalWorkflow_InvalidUUID(t *testing.T) {
	h := adaptgrpc.NewHandler(&mockClientService{}, &mockWorkflowTaskService{}, &mockActivityTaskService{})
	_, err := h.SignalWorkflow(context.Background(), &apiv1.SignalWorkflowRequest{
		WorkflowId: "not-a-uuid",
		SignalName: "test",
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestSignalWorkflow_NotRunning(t *testing.T) {
	h := adaptgrpc.NewHandler(
		&mockClientService{SignalWorkflowFn: func(_ context.Context, _ uuid.UUID, _ string, _ json.RawMessage) error {
			return domain.ErrWorkflowNotRunning
		}},
		&mockWorkflowTaskService{},
		&mockActivityTaskService{},
	)
	_, err := h.SignalWorkflow(context.Background(), &apiv1.SignalWorkflowRequest{
		WorkflowId: uuid.New().String(),
		SignalName: "test",
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("got code %v, want FailedPrecondition", st.Code())
	}
}

func TestCompleteWorkflowTask_InvalidCommand(t *testing.T) {
	h := adaptgrpc.NewHandler(&mockClientService{}, &mockWorkflowTaskService{}, &mockActivityTaskService{})
	_, err := h.CompleteWorkflowTask(context.Background(), &apiv1.CompleteWorkflowTaskRequest{
		TaskId: 1,
		Commands: []*apiv1.Command{
			{Type: apiv1.CommandType_COMMAND_TYPE_UNSPECIFIED, Attributes: []byte(`{}`)},
		},
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}
