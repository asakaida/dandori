package e2e_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_ThreeStepSequentialActivity(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "SequentialWorkflow",
		TaskQueue:    "seq-queue",
		Input:        []byte(`{"steps":3}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Step 1: PollWT -> ScheduleActivity(seq=1, "Step1")
	wt1 := pollWorkflowTaskUntil(t, ctx, "seq-queue", "worker-1", 5*time.Second)
	require.NotZero(t, wt1.GetTaskId())
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt1.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(1, "Step1", json.RawMessage(`{"step":1}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	// Step 1: PollAT -> CompleteAT
	at1 := pollActivityTaskUntil(t, ctx, "seq-queue", "worker-1", 5*time.Second)
	assert.Equal(t, "Step1", at1.GetActivityType())
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at1.GetTaskId(),
		Result: []byte(`{"out":"step1-done"}`),
	})
	require.NoError(t, err)

	// Step 2: PollWT -> ScheduleActivity(seq=2, "Step2")
	wt2 := pollWorkflowTaskUntil(t, ctx, "seq-queue", "worker-1", 5*time.Second)
	require.NotZero(t, wt2.GetTaskId())
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt2.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(2, "Step2", json.RawMessage(`{"step":2}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	// Step 2: PollAT -> CompleteAT
	at2 := pollActivityTaskUntil(t, ctx, "seq-queue", "worker-1", 5*time.Second)
	assert.Equal(t, "Step2", at2.GetActivityType())
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at2.GetTaskId(),
		Result: []byte(`{"out":"step2-done"}`),
	})
	require.NoError(t, err)

	// Step 3: PollWT -> ScheduleActivity(seq=3, "Step3")
	wt3 := pollWorkflowTaskUntil(t, ctx, "seq-queue", "worker-1", 5*time.Second)
	require.NotZero(t, wt3.GetTaskId())
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt3.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(3, "Step3", json.RawMessage(`{"step":3}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	// Step 3: PollAT -> CompleteAT
	at3 := pollActivityTaskUntil(t, ctx, "seq-queue", "worker-1", 5*time.Second)
	assert.Equal(t, "Step3", at3.GetActivityType())
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at3.GetTaskId(),
		Result: []byte(`{"out":"step3-done"}`),
	})
	require.NoError(t, err)

	// Final: PollWT -> CompleteWorkflow
	wt4 := pollWorkflowTaskUntil(t, ctx, "seq-queue", "worker-1", 5*time.Second)
	require.NotZero(t, wt4.GetTaskId())
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt4.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`{"final":"all-done"}`))},
	})
	require.NoError(t, err)

	// Verify: DescribeWorkflow == COMPLETED
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, desc.GetWorkflowExecution().GetStatus())

	// Verify: GetWorkflowHistory has expected events
	// Started + 3*(Scheduled+Completed) + WorkflowCompleted = 8 events
	hist, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)
	events := hist.GetEvents()
	assert.Equal(t, 8, len(events), "expected 8 history events, got %d", len(events))
	assert.NotNil(t, findEvent(events, "WorkflowExecutionStarted"))
	assert.Equal(t, 3, countEvents(events, "ActivityTaskScheduled"))
	assert.Equal(t, 3, countEvents(events, "ActivityTaskCompleted"))
	assert.NotNil(t, findEvent(events, "WorkflowExecutionCompleted"))
}

func TestE2E_ResultRetrieval(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "SimpleWorkflow",
		TaskQueue:    "result-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// PollWT -> CompleteWorkflow with specific result
	wt := pollWorkflowTaskUntil(t, ctx, "result-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`{"answer":42}`))},
	})
	require.NoError(t, err)

	// Verify result
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, desc.GetWorkflowExecution().GetStatus())

	var result map[string]any
	require.NoError(t, json.Unmarshal(desc.GetWorkflowExecution().GetResult(), &result))
	assert.Equal(t, float64(42), result["answer"])
}
