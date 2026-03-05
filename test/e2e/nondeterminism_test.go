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

func TestE2E_NonDeterminismFailWorkflowTask(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "NonDetWorkflow",
		TaskQueue:    "nondet-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// First WT: schedule activity
	wt1 := pollWorkflowTaskUntil(t, ctx, "nondet-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt1.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(1, "Step1", json.RawMessage(`{}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	// Complete activity
	at := pollActivityTaskUntil(t, ctx, "nondet-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"done":true}`),
	})
	require.NoError(t, err)

	// Second WT (replay with full history) - worker detects non-determinism
	wt2 := pollWorkflowTaskUntil(t, ctx, "nondet-queue", "worker-1", 5*time.Second)
	require.NotZero(t, wt2.GetTaskId())

	// Fail the WT with non-determinism error
	_, err = client.FailWorkflowTask(ctx, &apiv1.FailWorkflowTaskRequest{
		TaskId:  wt2.GetTaskId(),
		Cause:   "NonDeterminismError",
		Message: "expected ScheduleActivity(Step1) but got different command during replay",
	})
	require.NoError(t, err)

	// Verify workflow is FAILED
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED, desc.GetWorkflowExecution().GetStatus())

	// Verify history contains WorkflowExecutionFailed event
	hist, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.NotNil(t, findEvent(hist.GetEvents(), "WorkflowExecutionFailed"))
}
