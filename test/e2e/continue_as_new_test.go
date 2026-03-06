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

func TestE2E_ContinueAsNew_Manual(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "continue-wf",
		TaskQueue:    "default",
		Input:        []byte(`{"iteration":1}`),
	})
	require.NoError(t, err)
	oldWFID := startResp.GetWorkflowId()

	// Poll and ContinueAsNew
	wt := pollWorkflowTaskUntil(t, ctx, "default", "w1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
		Commands: []*apiv1.Command{
			continueAsNewCmd(json.RawMessage(`{"iteration":2}`), "", ""),
		},
	})
	require.NoError(t, err)

	// Old workflow should be CONTINUED_AS_NEW
	descResp, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{
		WorkflowId: oldWFID,
	})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW, descResp.GetWorkflowExecution().GetStatus())
	newWFID := descResp.GetWorkflowExecution().GetContinuedAsNewId()
	require.NotEmpty(t, newWFID)

	// New workflow should be RUNNING
	newDescResp, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{
		WorkflowId: newWFID,
	})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_RUNNING, newDescResp.GetWorkflowExecution().GetStatus())

	// Poll new workflow task and complete it
	wt2 := pollWorkflowTaskUntil(t, ctx, "default", "w1", 5*time.Second)
	assert.Equal(t, newWFID, wt2.GetWorkflowId())
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt2.GetTaskId(),
		Commands: []*apiv1.Command{
			completeWorkflowCmd(json.RawMessage(`{"result":"done"}`)),
		},
	})
	require.NoError(t, err)

	// New workflow should be COMPLETED
	finalDesc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{
		WorkflowId: newWFID,
	})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, finalDesc.GetWorkflowExecution().GetStatus())
}

func TestE2E_ContinueAsNew_InheritsWorkflowType(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "typed-wf",
		TaskQueue:    "default",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	oldWFID := startResp.GetWorkflowId()

	wt := pollWorkflowTaskUntil(t, ctx, "default", "w1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
		Commands: []*apiv1.Command{
			continueAsNewCmd(json.RawMessage(`{}`), "", ""),
		},
	})
	require.NoError(t, err)

	descResp, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{
		WorkflowId: oldWFID,
	})
	require.NoError(t, err)
	newWFID := descResp.GetWorkflowExecution().GetContinuedAsNewId()
	require.NotEmpty(t, newWFID)

	newDescResp, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{
		WorkflowId: newWFID,
	})
	require.NoError(t, err)
	assert.Equal(t, "typed-wf", newDescResp.GetWorkflowExecution().GetWorkflowType())
	assert.Equal(t, "default", newDescResp.GetWorkflowExecution().GetTaskQueue())
}
