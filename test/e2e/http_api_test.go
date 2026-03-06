package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func httpPost(t *testing.T, path string, body any) (int, map[string]any) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewReader(b)
	}
	resp, err := http.Post(httpServer.URL+path, "application/json", reqBody)
	require.NoError(t, err)
	defer resp.Body.Close()
	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		result = nil
	}
	return resp.StatusCode, result
}

func httpGet(t *testing.T, path string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(httpServer.URL + path)
	require.NoError(t, err)
	defer resp.Body.Close()
	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		result = nil
	}
	return resp.StatusCode, result
}

func TestHTTP_StartWorkflow(t *testing.T) {
	truncateAll(t)

	status, body := httpPost(t, "/v1/workflows", map[string]any{
		"workflow_type": "HttpTestWorkflow",
		"task_queue":    "http-test-queue",
	})

	assert.Equal(t, http.StatusOK, status)
	assert.NotEmpty(t, body["workflowId"])
}

func TestHTTP_StartAndDescribeWorkflow(t *testing.T) {
	truncateAll(t)

	// Start via HTTP
	status, body := httpPost(t, "/v1/workflows", map[string]any{
		"workflow_type": "HttpDescribeWorkflow",
		"task_queue":    "http-test-queue",
	})
	require.Equal(t, http.StatusOK, status)
	wfID := body["workflowId"].(string)

	// Describe via HTTP
	status, body = httpGet(t, "/v1/workflows/"+wfID)
	require.Equal(t, http.StatusOK, status)

	wfExec := body["workflowExecution"].(map[string]any)
	assert.Equal(t, wfID, wfExec["id"])
	assert.Equal(t, "HttpDescribeWorkflow", wfExec["workflowType"])
	assert.Equal(t, "http-test-queue", wfExec["taskQueue"])
	assert.Equal(t, "WORKFLOW_EXECUTION_STATUS_RUNNING", wfExec["status"])
}

func TestHTTP_DescribeWorkflow_NotFound(t *testing.T) {
	truncateAll(t)

	status, body := httpGet(t, "/v1/workflows/00000000-0000-0000-0000-000000000000")
	assert.Equal(t, http.StatusNotFound, status)
	assert.NotNil(t, body["message"])
}

func TestHTTP_TerminateWorkflow(t *testing.T) {
	truncateAll(t)

	// Start via gRPC
	ctx := context.Background()
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "HttpTerminateWorkflow",
		TaskQueue:    "http-test-queue",
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Terminate via HTTP
	status, _ := httpPost(t, "/v1/workflows/"+wfID+"/termination", map[string]any{
		"reason": "http terminate test",
	})
	assert.Equal(t, http.StatusOK, status)

	// Verify via HTTP
	status, body := httpGet(t, "/v1/workflows/"+wfID)
	require.Equal(t, http.StatusOK, status)
	wfExec := body["workflowExecution"].(map[string]any)
	assert.Equal(t, "WORKFLOW_EXECUTION_STATUS_TERMINATED", wfExec["status"])
}

func TestHTTP_GetWorkflowHistory(t *testing.T) {
	truncateAll(t)

	ctx := context.Background()
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "HttpHistoryWorkflow",
		TaskQueue:    "http-test-queue",
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Get history via HTTP
	status, body := httpGet(t, "/v1/workflows/"+wfID+"/history")
	require.Equal(t, http.StatusOK, status)

	events := body["events"].([]any)
	assert.GreaterOrEqual(t, len(events), 1)
	firstEvent := events[0].(map[string]any)
	assert.Equal(t, "WorkflowExecutionStarted", firstEvent["eventType"])
}

func TestHTTP_ListWorkflows(t *testing.T) {
	truncateAll(t)

	ctx := context.Background()
	for i := range 3 {
		_, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
			WorkflowType: fmt.Sprintf("HttpListWorkflow%d", i),
			TaskQueue:    "http-test-queue",
		})
		require.NoError(t, err)
	}

	// List via HTTP
	status, body := httpGet(t, "/v1/workflows?page_size=10")
	require.Equal(t, http.StatusOK, status)

	workflows := body["workflows"].([]any)
	assert.Equal(t, 3, len(workflows))
}

func TestHTTP_ListWorkflows_WithFilter(t *testing.T) {
	truncateAll(t)

	ctx := context.Background()
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "HttpFilterWorkflow",
		TaskQueue:    "http-test-queue",
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Terminate to make it non-running
	_, err = client.TerminateWorkflow(ctx, &apiv1.TerminateWorkflowRequest{
		WorkflowId: wfID,
		Reason:     "test",
	})
	require.NoError(t, err)

	// Start a running one
	_, err = client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "HttpFilterWorkflow2",
		TaskQueue:    "http-test-queue",
	})
	require.NoError(t, err)

	// Filter by RUNNING status
	status, body := httpGet(t, "/v1/workflows?status_filter=RUNNING")
	require.Equal(t, http.StatusOK, status)

	workflows := body["workflows"].([]any)
	assert.Equal(t, 1, len(workflows))
}

func TestHTTP_FullWorkflowLifecycle(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow via HTTP
	status, body := httpPost(t, "/v1/workflows", map[string]any{
		"workflow_type": "HttpLifecycleWorkflow",
		"task_queue":    "http-lifecycle-queue",
	})
	require.Equal(t, http.StatusOK, status)
	wfID := body["workflowId"].(string)

	// Poll and complete workflow via gRPC (worker simulation)
	wt := pollWorkflowTaskUntil(t, ctx, "http-lifecycle-queue", "w1", 5*time.Second)
	_, err := client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`"http-result"`))},
	})
	require.NoError(t, err)

	// Describe via HTTP to verify completed
	status, body = httpGet(t, "/v1/workflows/"+wfID)
	require.Equal(t, http.StatusOK, status)
	wfExec := body["workflowExecution"].(map[string]any)
	assert.Equal(t, "WORKFLOW_EXECUTION_STATUS_COMPLETED", wfExec["status"])
}
