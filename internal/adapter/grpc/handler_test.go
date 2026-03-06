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
					&mockClientService{DescribeWorkflowFn: func(_ context.Context, _ string, _ uuid.UUID) (*domain.WorkflowExecution, error) {
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
					&mockClientService{TerminateWorkflowFn: func(_ context.Context, _ string, _ uuid.UUID, _ string) error {
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
					&mockClientService{DescribeWorkflowFn: func(_ context.Context, _ string, _ uuid.UUID) (*domain.WorkflowExecution, error) {
						return nil, errors.New("something unexpected")
					}},
					&mockWorkflowTaskService{},
					&mockActivityTaskService{},
				)
			case "wrapped_error→unwrapped":
				h = adaptgrpc.NewHandler(
					&mockClientService{DescribeWorkflowFn: func(_ context.Context, _ string, _ uuid.UUID) (*domain.WorkflowExecution, error) {
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
		PollWorkflowTaskFn: func(_ context.Context, _ string, _ string, _ string) (*port.WorkflowTaskResult, error) {
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
		PollActivityTaskFn: func(_ context.Context, _ string, _ string, _ string) (*domain.ActivityTask, error) {
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
		&mockClientService{SignalWorkflowFn: func(_ context.Context, _ string, _ uuid.UUID, _ string, _ json.RawMessage) error {
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

func TestCancelWorkflow_InvalidUUID(t *testing.T) {
	h := adaptgrpc.NewHandler(&mockClientService{}, &mockWorkflowTaskService{}, &mockActivityTaskService{})
	_, err := h.CancelWorkflow(context.Background(), &apiv1.CancelWorkflowRequest{
		WorkflowId: "not-a-uuid",
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestCancelWorkflow_NotRunning(t *testing.T) {
	h := adaptgrpc.NewHandler(
		&mockClientService{CancelWorkflowFn: func(_ context.Context, _ string, _ uuid.UUID) error {
			return domain.ErrWorkflowNotRunning
		}},
		&mockWorkflowTaskService{},
		&mockActivityTaskService{},
	)
	_, err := h.CancelWorkflow(context.Background(), &apiv1.CancelWorkflowRequest{
		WorkflowId: uuid.New().String(),
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("got code %v, want FailedPrecondition", st.Code())
	}
}

func TestRecordActivityHeartbeat_TaskNotFound(t *testing.T) {
	h := adaptgrpc.NewHandler(
		&mockClientService{},
		&mockWorkflowTaskService{},
		&mockActivityTaskService{RecordActivityHeartbeatFn: func(_ context.Context, _ int64, _ json.RawMessage) error {
			return domain.ErrTaskNotFound
		}},
	)
	_, err := h.RecordActivityHeartbeat(context.Background(), &apiv1.RecordActivityHeartbeatRequest{
		TaskId:  999,
		Details: []byte(`{}`),
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("got code %v, want NotFound", st.Code())
	}
}

func TestListWorkflows_Success(t *testing.T) {
	wfID := uuid.New()
	h := adaptgrpc.NewHandler(
		&mockClientService{ListWorkflowsFn: func(_ context.Context, params port.ListWorkflowsParams) (*port.ListWorkflowsResult, error) {
			return &port.ListWorkflowsResult{
				Workflows: []domain.WorkflowExecution{
					{ID: wfID, WorkflowType: "TestWF", TaskQueue: "q", Status: domain.WorkflowStatusRunning},
				},
			}, nil
		}},
		&mockWorkflowTaskService{},
		&mockActivityTaskService{},
	)
	resp, err := h.ListWorkflows(context.Background(), &apiv1.ListWorkflowsRequest{PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetWorkflows()) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(resp.GetWorkflows()))
	}
	if resp.GetWorkflows()[0].GetId() != wfID.String() {
		t.Errorf("got id %s, want %s", resp.GetWorkflows()[0].GetId(), wfID.String())
	}
}

func TestListWorkflows_InvalidToken(t *testing.T) {
	h := adaptgrpc.NewHandler(&mockClientService{}, &mockWorkflowTaskService{}, &mockActivityTaskService{})
	_, err := h.ListWorkflows(context.Background(), &apiv1.ListWorkflowsRequest{
		NextPageToken: "not-valid-base64!!!",
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestQueryWorkflow_InvalidUUID(t *testing.T) {
	h := adaptgrpc.NewHandler(&mockClientService{}, &mockWorkflowTaskService{}, &mockActivityTaskService{})
	_, err := h.QueryWorkflow(context.Background(), &apiv1.QueryWorkflowRequest{
		WorkflowId: "not-a-uuid",
		QueryType:  "getState",
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestRespondQueryTask_Success(t *testing.T) {
	var called bool
	h := adaptgrpc.NewHandler(
		&mockClientService{},
		&mockWorkflowTaskService{
			RespondQueryTaskFn: func(_ context.Context, queryID int64, result json.RawMessage, errMsg string) error {
				called = true
				if queryID != 42 {
					t.Errorf("got queryID %d, want 42", queryID)
				}
				return nil
			},
		},
		&mockActivityTaskService{},
	)
	_, err := h.RespondQueryTask(context.Background(), &apiv1.RespondQueryTaskRequest{
		QueryId: 42,
		Result:  []byte(`"ok"`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("RespondQueryTask was not called")
	}
}

func TestCommandTypeFromProto_ContinueAsNew(t *testing.T) {
	h := adaptgrpc.NewHandler(&mockClientService{}, &mockWorkflowTaskService{
		CompleteWorkflowTaskFn: func(_ context.Context, _ int64, commands []domain.Command) error {
			if len(commands) != 1 {
				t.Fatalf("expected 1 command, got %d", len(commands))
			}
			if commands[0].Type != domain.CommandContinueAsNew {
				t.Errorf("got command type %s, want ContinueAsNew", commands[0].Type)
			}
			return nil
		},
	}, &mockActivityTaskService{})

	_, err := h.CompleteWorkflowTask(context.Background(), &apiv1.CompleteWorkflowTaskRequest{
		TaskId: 1,
		Commands: []*apiv1.Command{
			{Type: apiv1.CommandType_COMMAND_TYPE_CONTINUE_AS_NEW, Attributes: []byte(`{"input":"{}"}`)},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkflowStatusToProto_ContinuedAsNew(t *testing.T) {
	wfID := uuid.New()
	h := adaptgrpc.NewHandler(
		&mockClientService{DescribeWorkflowFn: func(_ context.Context, _ string, _ uuid.UUID) (*domain.WorkflowExecution, error) {
			return &domain.WorkflowExecution{
				ID:           wfID,
				WorkflowType: "wf",
				TaskQueue:    "q",
				Status:       domain.WorkflowStatusContinuedAsNew,
				CronSchedule: "* * * * *",
			}, nil
		}},
		&mockWorkflowTaskService{},
		&mockActivityTaskService{},
	)

	resp, err := h.DescribeWorkflow(context.Background(), &apiv1.DescribeWorkflowRequest{
		WorkflowId: wfID.String(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetWorkflowExecution().GetStatus() != apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW {
		t.Errorf("got status %v, want CONTINUED_AS_NEW", resp.GetWorkflowExecution().GetStatus())
	}
	if resp.GetWorkflowExecution().GetCronSchedule() != "* * * * *" {
		t.Errorf("got cron_schedule %q, want %q", resp.GetWorkflowExecution().GetCronSchedule(), "* * * * *")
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
