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

func TestE2E_CronWorkflow_AutoRestart(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow with cron schedule
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "cron-wf",
		TaskQueue:    "default",
		Input:        []byte(`{"run":1}`),
		CronSchedule: "* * * * *",
	})
	require.NoError(t, err)
	wfID1 := startResp.GetWorkflowId()

	// Poll and complete first run
	wt1 := pollWorkflowTaskUntil(t, ctx, "default", "w1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt1.GetTaskId(),
		Commands: []*apiv1.Command{
			completeWorkflowCmd(json.RawMessage(`{"run":1,"done":true}`)),
		},
	})
	require.NoError(t, err)

	// First workflow should be CONTINUED_AS_NEW
	desc1, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{
		WorkflowId: wfID1,
	})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW, desc1.GetWorkflowExecution().GetStatus())
	wfID2 := desc1.GetWorkflowExecution().GetContinuedAsNewId()
	require.NotEmpty(t, wfID2)

	// Second workflow should be RUNNING with cron schedule inherited
	desc2, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{
		WorkflowId: wfID2,
	})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_RUNNING, desc2.GetWorkflowExecution().GetStatus())
	assert.Equal(t, "* * * * *", desc2.GetWorkflowExecution().GetCronSchedule())

	// Complete second run - should auto-restart again
	wt2 := pollWorkflowTaskUntil(t, ctx, "default", "w1", 5*time.Second)
	assert.Equal(t, wfID2, wt2.GetWorkflowId())
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt2.GetTaskId(),
		Commands: []*apiv1.Command{
			completeWorkflowCmd(json.RawMessage(`{"run":2,"done":true}`)),
		},
	})
	require.NoError(t, err)

	// Second workflow should also be CONTINUED_AS_NEW
	desc2After, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{
		WorkflowId: wfID2,
	})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW, desc2After.GetWorkflowExecution().GetStatus())
	wfID3 := desc2After.GetWorkflowExecution().GetContinuedAsNewId()
	require.NotEmpty(t, wfID3)

	// Third workflow should be RUNNING
	desc3, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{
		WorkflowId: wfID3,
	})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_RUNNING, desc3.GetWorkflowExecution().GetStatus())
}

func TestE2E_CronWorkflow_FailDoesNotRestart(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "cron-wf",
		TaskQueue:    "default",
		Input:        []byte(`{}`),
		CronSchedule: "* * * * *",
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll and fail the workflow
	wt := pollWorkflowTaskUntil(t, ctx, "default", "w1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
		Commands: []*apiv1.Command{
			failWorkflowCmd("intentional failure"),
		},
	})
	require.NoError(t, err)

	// Workflow should be FAILED, not CONTINUED_AS_NEW
	desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{
		WorkflowId: wfID,
	})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED, desc.GetWorkflowExecution().GetStatus())
}
