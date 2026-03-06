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

func TestE2E_SignalWorkflow(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "SignalWorkflow",
		TaskQueue:    "signal-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll initial WT and complete with no commands (waiting for signal)
	wt := pollWorkflowTaskUntil(t, ctx, "signal-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
	})
	require.NoError(t, err)

	// Send signal
	_, err = client.SignalWorkflow(ctx, &apiv1.SignalWorkflowRequest{
		WorkflowId: wfID,
		SignalName: "approval",
		Input:      []byte(`{"approved":true}`),
	})
	require.NoError(t, err)

	// Poll new WT triggered by signal
	wt2 := pollWorkflowTaskUntil(t, ctx, "signal-queue", "worker-1", 5*time.Second)
	require.NotZero(t, wt2.GetTaskId())

	// Verify history contains WorkflowSignaled event
	var hasSignal bool
	for _, e := range wt2.GetEvents() {
		if e.GetEventType() == "WorkflowSignaled" {
			hasSignal = true
			var data map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(e.GetEventData(), &data))
			assert.JSONEq(t, `"approval"`, string(data["signal_name"]))
		}
	}
	assert.True(t, hasSignal, "expected WorkflowSignaled event in history")

	// Complete workflow
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt2.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`{"done":true}`))},
	})
	require.NoError(t, err)

	// Verify COMPLETED
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, desc.GetWorkflowExecution().GetStatus())

	// Verify full history
	hist, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.NotNil(t, findEvent(hist.GetEvents(), "WorkflowSignaled"))
	assert.NotNil(t, findEvent(hist.GetEvents(), "WorkflowExecutionCompleted"))
}

func TestE2E_SignalWorkflow_NotRunning(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start and complete a workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "QuickWorkflow",
		TaskQueue:    "signal-nr-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	wt := pollWorkflowTaskUntil(t, ctx, "signal-nr-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`{}`))},
	})
	require.NoError(t, err)

	// Signal completed workflow -> should fail
	_, err = client.SignalWorkflow(ctx, &apiv1.SignalWorkflowRequest{
		WorkflowId: wfID,
		SignalName: "late-signal",
		Input:      []byte(`{}`),
	})
	require.Error(t, err)
}

func TestE2E_SignalWorkflow_MultipleSignals(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "MultiSignalWorkflow",
		TaskQueue:    "multi-signal-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll initial WT
	wt := pollWorkflowTaskUntil(t, ctx, "multi-signal-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
	})
	require.NoError(t, err)

	// Send 3 signals
	for i := 0; i < 3; i++ {
		_, err = client.SignalWorkflow(ctx, &apiv1.SignalWorkflowRequest{
			WorkflowId: wfID,
			SignalName: "data",
			Input:      []byte(`{"seq":` + string(rune('0'+i)) + `}`),
		})
		require.NoError(t, err)
	}

	// Drain all workflow tasks (one per signal)
	for i := 0; i < 3; i++ {
		wt := pollWorkflowTaskUntil(t, ctx, "multi-signal-queue", "worker-1", 5*time.Second)
		_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
			TaskId: wt.GetTaskId(),
		})
		require.NoError(t, err)
	}

	// Verify history has exactly 3 WorkflowSignaled events
	hist, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, 3, countEvents(hist.GetEvents(), "WorkflowSignaled"), "expected 3 WorkflowSignaled events in history")
}
