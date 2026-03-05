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

func TestE2E_ActivityTimeout(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "TimeoutWorkflow",
		TaskQueue:    "timeout-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll WT -> Schedule activity with short timeout (1s)
	wt1 := pollWorkflowTaskUntil(t, ctx, "timeout-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt1.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(1, "SlowActivity", json.RawMessage(`{}`), 1*time.Second, nil)},
	})
	require.NoError(t, err)

	// Poll AT to get the task (sets timeout_at in DB), but DO NOT complete it
	at := pollActivityTaskUntil(t, ctx, "timeout-queue", "worker-1", 5*time.Second)
	require.NotZero(t, at.GetTaskId())
	// Intentionally not completing — let the timeout fire

	// Wait for BackgroundWorker timeout checker (500ms interval) to detect timeout
	// Activity timeout = 1s, checker interval = 500ms → should fire within ~2s
	// Then a new workflow task is enqueued

	// Poll for the new WT triggered by timeout
	wt2 := pollWorkflowTaskUntil(t, ctx, "timeout-queue", "worker-1", 10*time.Second)
	require.NotZero(t, wt2.GetTaskId())

	// Verify events contain ActivityTaskTimedOut
	assert.NotNil(t, findEvent(wt2.GetEvents(), "ActivityTaskTimedOut"), "expected ActivityTaskTimedOut event in WT events")

	// Complete or fail the workflow after timeout
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt2.GetTaskId(),
		Commands: []*apiv1.Command{failWorkflowCmd("activity timed out")},
	})
	require.NoError(t, err)

	// Verify FAILED
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED, desc.GetWorkflowExecution().GetStatus())

	// Verify history
	hist, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: wfID})
	require.NoError(t, err)
	assert.NotNil(t, findEvent(hist.GetEvents(), "ActivityTaskTimedOut"))
	assert.NotNil(t, findEvent(hist.GetEvents(), "WorkflowExecutionFailed"))
}
