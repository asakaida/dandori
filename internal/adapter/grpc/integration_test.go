package grpc_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIntegration_StartWorkflow_PollComplete(t *testing.T) {
	if testDB == nil {
		t.Skip("no test database")
	}
	h := newTestHandler(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := h.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "TestWorkflow",
		TaskQueue:    "test-queue",
		Input:        []byte(`{"key":"value"}`),
	})
	if err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}
	wfID := startResp.GetWorkflowId()

	// Poll workflow task
	pollResp, err := h.PollWorkflowTask(ctx, &apiv1.PollWorkflowTaskRequest{
		QueueName: "test-queue",
		WorkerId:  "worker-1",
	})
	if err != nil {
		t.Fatalf("PollWorkflowTask: %v", err)
	}
	if pollResp.GetTaskId() == 0 {
		t.Fatal("expected a workflow task")
	}
	if pollResp.GetWorkflowId() != wfID {
		t.Errorf("got workflow_id %s, want %s", pollResp.GetWorkflowId(), wfID)
	}
	if pollResp.GetWorkflowType() != "TestWorkflow" {
		t.Errorf("got workflow_type %s, want TestWorkflow", pollResp.GetWorkflowType())
	}

	// Complete workflow task with CompleteWorkflow command
	result, _ := json.Marshal(map[string]string{"result": "done"})
	attrs, _ := json.Marshal(map[string]json.RawMessage{"result": result})
	_, err = h.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: pollResp.GetTaskId(),
		Commands: []*apiv1.Command{
			{Type: apiv1.CommandType_COMMAND_TYPE_COMPLETE_WORKFLOW, Attributes: attrs},
		},
	})
	if err != nil {
		t.Fatalf("CompleteWorkflowTask: %v", err)
	}

	// Describe workflow — should be COMPLETED
	descResp, err := h.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	if err != nil {
		t.Fatalf("DescribeWorkflow: %v", err)
	}
	if descResp.GetWorkflowExecution().GetStatus() != apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED {
		t.Errorf("got status %v, want COMPLETED", descResp.GetWorkflowExecution().GetStatus())
	}
}

func TestIntegration_ActivityFlow(t *testing.T) {
	if testDB == nil {
		t.Skip("no test database")
	}
	h := newTestHandler(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := h.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "ActivityWorkflow",
		TaskQueue:    "test-queue",
		Input:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}
	wfID := startResp.GetWorkflowId()

	// Poll workflow task
	pollWT1, err := h.PollWorkflowTask(ctx, &apiv1.PollWorkflowTaskRequest{
		QueueName: "test-queue",
		WorkerId:  "worker-1",
	})
	if err != nil {
		t.Fatalf("PollWorkflowTask: %v", err)
	}

	// Complete WT with ScheduleActivity
	schedAttrs, _ := json.Marshal(map[string]any{
		"seq_id":                 1,
		"activity_type":         "MyActivity",
		"input":                 json.RawMessage(`{"x":1}`),
		"start_to_close_timeout": 30000000000,
	})
	_, err = h.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: pollWT1.GetTaskId(),
		Commands: []*apiv1.Command{
			{Type: apiv1.CommandType_COMMAND_TYPE_SCHEDULE_ACTIVITY_TASK, Attributes: schedAttrs},
		},
	})
	if err != nil {
		t.Fatalf("CompleteWorkflowTask (schedule): %v", err)
	}

	// Poll activity task
	pollAT, err := h.PollActivityTask(ctx, &apiv1.PollActivityTaskRequest{
		QueueName: "test-queue",
		WorkerId:  "worker-1",
	})
	if err != nil {
		t.Fatalf("PollActivityTask: %v", err)
	}
	if pollAT.GetTaskId() == 0 {
		t.Fatal("expected an activity task")
	}
	if pollAT.GetActivityType() != "MyActivity" {
		t.Errorf("got activity_type %s, want MyActivity", pollAT.GetActivityType())
	}

	// Complete activity task
	_, err = h.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: pollAT.GetTaskId(),
		Result: []byte(`{"out":"ok"}`),
	})
	if err != nil {
		t.Fatalf("CompleteActivityTask: %v", err)
	}

	// Brief pause to ensure scheduled_at <= NOW() in DB
	time.Sleep(50 * time.Millisecond)

	// Poll second workflow task (triggered by activity completion)
	pollWT2, err := h.PollWorkflowTask(ctx, &apiv1.PollWorkflowTaskRequest{
		QueueName: "test-queue",
		WorkerId:  "worker-1",
	})
	if err != nil {
		t.Fatalf("PollWorkflowTask (2nd): %v", err)
	}
	if pollWT2.GetTaskId() == 0 {
		t.Fatal("expected a second workflow task")
	}

	// Complete workflow with CompleteWorkflow
	cwAttrs, _ := json.Marshal(map[string]json.RawMessage{"result": json.RawMessage(`{"final":"result"}`)})
	_, err = h.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: pollWT2.GetTaskId(),
		Commands: []*apiv1.Command{
			{Type: apiv1.CommandType_COMMAND_TYPE_COMPLETE_WORKFLOW, Attributes: cwAttrs},
		},
	})
	if err != nil {
		t.Fatalf("CompleteWorkflowTask (complete): %v", err)
	}

	// Describe — COMPLETED
	descResp, err := h.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	if err != nil {
		t.Fatalf("DescribeWorkflow: %v", err)
	}
	if descResp.GetWorkflowExecution().GetStatus() != apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED {
		t.Errorf("got status %v, want COMPLETED", descResp.GetWorkflowExecution().GetStatus())
	}
}

func TestIntegration_TerminateWorkflow(t *testing.T) {
	if testDB == nil {
		t.Skip("no test database")
	}
	h := newTestHandler(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := h.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "TestWorkflow",
		TaskQueue:    "test-queue",
		Input:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}
	wfID := startResp.GetWorkflowId()

	// Terminate
	_, err = h.TerminateWorkflow(ctx, &apiv1.TerminateWorkflowRequest{
		WorkflowId: wfID,
		Reason:     "test termination",
	})
	if err != nil {
		t.Fatalf("TerminateWorkflow: %v", err)
	}

	// Describe — TERMINATED
	descResp, err := h.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	if err != nil {
		t.Fatalf("DescribeWorkflow: %v", err)
	}
	if descResp.GetWorkflowExecution().GetStatus() != apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_TERMINATED {
		t.Errorf("got status %v, want TERMINATED", descResp.GetWorkflowExecution().GetStatus())
	}

	// History should contain termination event
	histResp, err := h.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	if err != nil {
		t.Fatalf("GetWorkflowHistory: %v", err)
	}
	found := false
	for _, e := range histResp.GetEvents() {
		if e.GetEventType() == "WorkflowExecutionTerminated" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected WorkflowExecutionTerminated event in history")
	}
}

func TestIntegration_FailWorkflowTask(t *testing.T) {
	if testDB == nil {
		t.Skip("no test database")
	}
	h := newTestHandler(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := h.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "TestWorkflow",
		TaskQueue:    "test-queue",
		Input:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}
	wfID := startResp.GetWorkflowId()

	// Poll workflow task
	pollResp, err := h.PollWorkflowTask(ctx, &apiv1.PollWorkflowTaskRequest{
		QueueName: "test-queue",
		WorkerId:  "worker-1",
	})
	if err != nil {
		t.Fatalf("PollWorkflowTask: %v", err)
	}

	// Fail workflow task
	_, err = h.FailWorkflowTask(ctx, &apiv1.FailWorkflowTaskRequest{
		TaskId:  pollResp.GetTaskId(),
		Cause:   "panic",
		Message: "something crashed",
	})
	if err != nil {
		t.Fatalf("FailWorkflowTask: %v", err)
	}

	// Describe — FAILED
	descResp, err := h.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	if err != nil {
		t.Fatalf("DescribeWorkflow: %v", err)
	}
	if descResp.GetWorkflowExecution().GetStatus() != apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED {
		t.Errorf("got status %v, want FAILED", descResp.GetWorkflowExecution().GetStatus())
	}
}

func TestIntegration_StartWorkflow_Duplicate(t *testing.T) {
	if testDB == nil {
		t.Skip("no test database")
	}
	h := newTestHandler(t)
	ctx := context.Background()

	resp, err := h.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "TestWorkflow",
		TaskQueue:    "test-queue",
		Input:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}
	wfID := resp.GetWorkflowId()

	// Start again with same ID
	_, err = h.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowId:   wfID,
		WorkflowType: "TestWorkflow",
		TaskQueue:    "test-queue",
		Input:        []byte(`{}`),
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.AlreadyExists {
		t.Errorf("got code %v, want AlreadyExists", st.Code())
	}
}

func TestIntegration_DescribeWorkflow_NotFound(t *testing.T) {
	if testDB == nil {
		t.Skip("no test database")
	}
	h := newTestHandler(t)
	ctx := context.Background()

	_, err := h.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{
		WorkflowId: "00000000-0000-0000-0000-000000000001",
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("got code %v, want NotFound", st.Code())
	}
}

func TestIntegration_PollWorkflowTask_Empty(t *testing.T) {
	if testDB == nil {
		t.Skip("no test database")
	}
	h := newTestHandler(t)
	ctx := context.Background()

	resp, err := h.PollWorkflowTask(ctx, &apiv1.PollWorkflowTaskRequest{
		QueueName: "empty-queue",
		WorkerId:  "worker-1",
	})
	if err != nil {
		t.Fatalf("PollWorkflowTask: %v", err)
	}
	if resp.GetTaskId() != 0 {
		t.Errorf("expected empty response, got task_id=%d", resp.GetTaskId())
	}
}
