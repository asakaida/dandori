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

func TestE2E_CancelWorkflow(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "CancelableWorkflow",
		TaskQueue:    "cancel-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll initial WT and complete with no commands (waiting)
	wt := pollWorkflowTaskUntil(t, ctx, "cancel-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
	})
	require.NoError(t, err)

	// Cancel workflow
	_, err = client.CancelWorkflow(ctx, &apiv1.CancelWorkflowRequest{
		WorkflowId: wfID,
	})
	require.NoError(t, err)

	// Poll new WT triggered by cancel
	wt2 := pollWorkflowTaskUntil(t, ctx, "cancel-queue", "worker-1", 5*time.Second)
	require.NotZero(t, wt2.GetTaskId())

	// Verify history contains WorkflowCancelRequested event
	var hasCancelRequested bool
	for _, e := range wt2.GetEvents() {
		if e.GetEventType() == "WorkflowCancelRequested" {
			hasCancelRequested = true
		}
	}
	assert.True(t, hasCancelRequested, "expected WorkflowCancelRequested event in history")

	// Verify workflow is still RUNNING (graceful cancel)
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_RUNNING, desc.GetWorkflowExecution().GetStatus())

	// Complete workflow (acknowledging cancel)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt2.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`{"canceled":true}`))},
	})
	require.NoError(t, err)

	// Verify COMPLETED
	desc, err = client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, desc.GetWorkflowExecution().GetStatus())
}
