package e2e_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_ListWorkflows(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start 3 workflows
	for i := 0; i < 3; i++ {
		_, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
			WorkflowType: fmt.Sprintf("ListWF-%d", i),
			TaskQueue:    "list-queue",
			Input:        []byte(`{}`),
		})
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	resp, err := client.ListWorkflows(ctx, &apiv1.ListWorkflowsRequest{PageSize: 10})
	require.NoError(t, err)
	assert.Len(t, resp.GetWorkflows(), 3)

	// All should be RUNNING
	for _, wf := range resp.GetWorkflows() {
		assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_RUNNING, wf.GetStatus())
	}
}

func TestE2E_ListWorkflows_StatusFilter(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start and terminate one WF
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "FilterWF",
		TaskQueue:    "filter-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)

	_, err = client.TerminateWorkflow(ctx, &apiv1.TerminateWorkflowRequest{
		WorkflowId: startResp.GetWorkflowId(),
		Reason:     "test",
	})
	require.NoError(t, err)

	// Start another running WF
	_, err = client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "FilterWF",
		TaskQueue:    "filter-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)

	// Filter RUNNING only
	resp, err := client.ListWorkflows(ctx, &apiv1.ListWorkflowsRequest{
		PageSize:     10,
		StatusFilter: "RUNNING",
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetWorkflows(), 1)

	// Filter TERMINATED
	resp2, err := client.ListWorkflows(ctx, &apiv1.ListWorkflowsRequest{
		PageSize:     10,
		StatusFilter: "TERMINATED",
	})
	require.NoError(t, err)
	assert.Len(t, resp2.GetWorkflows(), 1)
}

func TestE2E_ListWorkflows_Pagination(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start 5 workflows
	for i := 0; i < 5; i++ {
		_, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
			WorkflowType: "PaginationWF",
			TaskQueue:    "page-queue",
			Input:        []byte(`{}`),
		})
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	// Page 1: 2 items
	page1, err := client.ListWorkflows(ctx, &apiv1.ListWorkflowsRequest{PageSize: 2})
	require.NoError(t, err)
	assert.Len(t, page1.GetWorkflows(), 2)
	assert.NotEmpty(t, page1.GetNextPageToken())

	// Page 2: 2 items
	page2, err := client.ListWorkflows(ctx, &apiv1.ListWorkflowsRequest{
		PageSize:      2,
		NextPageToken: page1.GetNextPageToken(),
	})
	require.NoError(t, err)
	assert.Len(t, page2.GetWorkflows(), 2)
	assert.NotEmpty(t, page2.GetNextPageToken())

	// Page 3: 1 item
	page3, err := client.ListWorkflows(ctx, &apiv1.ListWorkflowsRequest{
		PageSize:      2,
		NextPageToken: page2.GetNextPageToken(),
	})
	require.NoError(t, err)
	assert.Len(t, page3.GetWorkflows(), 1)
	assert.Empty(t, page3.GetNextPageToken())

	// Verify all IDs are unique
	allIDs := map[string]bool{}
	for _, wf := range page1.GetWorkflows() {
		allIDs[wf.GetId()] = true
	}
	for _, wf := range page2.GetWorkflows() {
		allIDs[wf.GetId()] = true
	}
	for _, wf := range page3.GetWorkflows() {
		allIDs[wf.GetId()] = true
	}
	assert.Len(t, allIDs, 5)
}
