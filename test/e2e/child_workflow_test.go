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

func TestE2E_ChildWorkflow_Completed(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start parent workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "ParentWorkflow",
		TaskQueue:    "child-wf-queue",
		Input:        []byte(`{"parent":true}`),
	})
	require.NoError(t, err)
	parentWFID := startResp.GetWorkflowId()

	// Parent: PollWT -> StartChildWorkflow
	wt := pollWorkflowTaskUntil(t, ctx, "child-wf-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{startChildWorkflowCmd(1, "ChildWorkflow", "", json.RawMessage(`{"child":true}`))},
	})
	require.NoError(t, err)

	// Child: PollWT -> ScheduleActivity
	childWT := pollWorkflowTaskUntil(t, ctx, "child-wf-queue", "worker-1", 5*time.Second)
	require.NotZero(t, childWT.GetTaskId())
	childWFID := childWT.GetWorkflowId()
	assert.NotEqual(t, parentWFID, childWFID)
	assert.Equal(t, "ChildWorkflow", childWT.GetWorkflowType())

	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   childWT.GetTaskId(),
		Commands: []*apiv1.Command{scheduleActivityCmd(1, "child-activity", json.RawMessage(`{}`), 30*time.Second, nil)},
	})
	require.NoError(t, err)

	// Child: PollAT -> CompleteAT
	at := pollActivityTaskUntil(t, ctx, "child-wf-queue", "worker-1", 5*time.Second)
	assert.Equal(t, "child-activity", at.GetActivityType())
	_, err = client.CompleteActivityTask(ctx, &apiv1.CompleteActivityTaskRequest{
		TaskId: at.GetTaskId(),
		Result: []byte(`{"child_result":"done"}`),
	})
	require.NoError(t, err)

	// Child: PollWT -> CompleteWorkflow
	childWT2 := pollWorkflowTaskUntil(t, ctx, "child-wf-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   childWT2.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`{"child_done":true}`))},
	})
	require.NoError(t, err)

	// Verify: child workflow is COMPLETED
	childDesc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: childWFID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, childDesc.GetWorkflowExecution().GetStatus())
	assert.Equal(t, parentWFID, childDesc.GetWorkflowExecution().GetParentWorkflowId())

	// Parent: PollWT (triggered by child completion) -> verify ChildWorkflowExecutionCompleted event -> CompleteWorkflow
	parentWT := pollWorkflowTaskUntil(t, ctx, "child-wf-queue", "worker-1", 5*time.Second)
	assert.Equal(t, parentWFID, parentWT.GetWorkflowId())

	// Verify parent has ChildWorkflowExecutionCompleted event
	childCompletedEvent := findEvent(parentWT.GetEvents(), "ChildWorkflowExecutionCompleted")
	require.NotNil(t, childCompletedEvent, "parent should have ChildWorkflowExecutionCompleted event")
	var completedData map[string]any
	require.NoError(t, json.Unmarshal(childCompletedEvent.GetEventData(), &completedData))
	assert.Equal(t, childWFID, completedData["child_workflow_id"])

	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   parentWT.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`{"parent_done":true}`))},
	})
	require.NoError(t, err)

	// Verify: parent workflow is COMPLETED
	parentDesc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: parentWFID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, parentDesc.GetWorkflowExecution().GetStatus())

	// Verify parent event history
	hist, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: parentWFID})
	require.NoError(t, err)
	events := hist.GetEvents()
	// WorkflowExecutionStarted + ChildWorkflowExecutionStarted + ChildWorkflowExecutionCompleted + WorkflowExecutionCompleted = 4
	assert.Equal(t, 4, len(events), "expected 4 parent history events, got %d", len(events))
	assert.NotNil(t, findEvent(events, "ChildWorkflowExecutionStarted"))
	assert.NotNil(t, findEvent(events, "ChildWorkflowExecutionCompleted"))
}

func TestE2E_ChildWorkflow_Failed(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start parent workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "ParentWorkflow",
		TaskQueue:    "child-fail-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	parentWFID := startResp.GetWorkflowId()

	// Parent: PollWT -> StartChildWorkflow
	wt := pollWorkflowTaskUntil(t, ctx, "child-fail-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{startChildWorkflowCmd(1, "FailingChildWorkflow", "", json.RawMessage(`{}`))},
	})
	require.NoError(t, err)

	// Child: PollWT -> FailWorkflow
	childWT := pollWorkflowTaskUntil(t, ctx, "child-fail-queue", "worker-1", 5*time.Second)
	childWFID := childWT.GetWorkflowId()
	assert.NotEqual(t, parentWFID, childWFID)

	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   childWT.GetTaskId(),
		Commands: []*apiv1.Command{failWorkflowCmd("child workflow error")},
	})
	require.NoError(t, err)

	// Verify: child workflow is FAILED
	childDesc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: childWFID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED, childDesc.GetWorkflowExecution().GetStatus())

	// Parent: PollWT (triggered by child failure) -> verify ChildWorkflowExecutionFailed event -> FailWorkflow
	parentWT := pollWorkflowTaskUntil(t, ctx, "child-fail-queue", "worker-1", 5*time.Second)
	assert.Equal(t, parentWFID, parentWT.GetWorkflowId())

	childFailedEvent := findEvent(parentWT.GetEvents(), "ChildWorkflowExecutionFailed")
	require.NotNil(t, childFailedEvent, "parent should have ChildWorkflowExecutionFailed event")
	var failedData map[string]any
	require.NoError(t, json.Unmarshal(childFailedEvent.GetEventData(), &failedData))
	assert.Equal(t, childWFID, failedData["child_workflow_id"])
	assert.Equal(t, "child workflow error", failedData["error_message"])

	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   parentWT.GetTaskId(),
		Commands: []*apiv1.Command{failWorkflowCmd("parent failed due to child failure")},
	})
	require.NoError(t, err)

	// Verify: parent workflow is FAILED
	parentDesc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: parentWFID})
	require.NoError(t, err)
	assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED, parentDesc.GetWorkflowExecution().GetStatus())

	// Verify parent event history
	hist, err := client.GetWorkflowHistory(ctx, &apiv1.GetWorkflowHistoryRequest{WorkflowId: parentWFID})
	require.NoError(t, err)
	events := hist.GetEvents()
	// WorkflowExecutionStarted + ChildWorkflowExecutionStarted + ChildWorkflowExecutionFailed + WorkflowExecutionFailed = 4
	assert.Equal(t, 4, len(events), "expected 4 parent history events, got %d", len(events))
	assert.NotNil(t, findEvent(events, "ChildWorkflowExecutionStarted"))
	assert.NotNil(t, findEvent(events, "ChildWorkflowExecutionFailed"))
	assert.NotNil(t, findEvent(events, "WorkflowExecutionFailed"))
}
