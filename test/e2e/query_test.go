package e2e_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestE2E_QueryWorkflow(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "QueryWorkflow",
		TaskQueue:    "query-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll initial WT and complete with activity (to keep WF running)
	wt := pollWorkflowTaskUntil(t, ctx, "query-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
		Commands: []*apiv1.Command{
			scheduleActivityCmd(1, "doWork", json.RawMessage(`{}`), 30*time.Second, nil),
		},
	})
	require.NoError(t, err)

	// Send query in a goroutine (it blocks until answered)
	type queryResult struct {
		resp *apiv1.QueryWorkflowResponse
		err  error
	}
	ch := make(chan queryResult, 1)
	go func() {
		resp, err := client.QueryWorkflow(ctx, &apiv1.QueryWorkflowRequest{
			WorkflowId: wfID,
			QueryType:  "getState",
			Input:      []byte(`{"key":"counter"}`),
		})
		ch <- queryResult{resp, err}
	}()

	// Poll WT triggered by query
	wt2 := pollWorkflowTaskUntil(t, ctx, "query-queue", "worker-1", 5*time.Second)
	require.NotZero(t, wt2.GetTaskId())
	require.NotEmpty(t, wt2.GetPendingQueries())

	pq := wt2.GetPendingQueries()[0]
	assert.Equal(t, "getState", pq.GetQueryType())
	assert.JSONEq(t, `{"key":"counter"}`, string(pq.GetInput()))

	// Respond to query
	_, err = client.RespondQueryTask(ctx, &apiv1.RespondQueryTaskRequest{
		QueryId: pq.GetQueryId(),
		Result:  []byte(`{"counter":42}`),
	})
	require.NoError(t, err)

	// Complete WT (no new commands - just completing the query-triggered WT)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt2.GetTaskId(),
	})
	require.NoError(t, err)

	// Wait for QueryWorkflow to return
	result := <-ch
	require.NoError(t, result.err)
	assert.JSONEq(t, `{"counter":42}`, string(result.resp.GetResult()))
	assert.Empty(t, result.resp.GetErrorMessage())
}

func TestE2E_QueryWorkflow_NotRunning(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start and immediately complete a workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "QuickWorkflow",
		TaskQueue:    "query-nr-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	wt := pollWorkflowTaskUntil(t, ctx, "query-nr-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId:   wt.GetTaskId(),
		Commands: []*apiv1.Command{completeWorkflowCmd(json.RawMessage(`{}`))},
	})
	require.NoError(t, err)

	// Query completed workflow -> should fail with FailedPrecondition
	_, err = client.QueryWorkflow(ctx, &apiv1.QueryWorkflowRequest{
		WorkflowId: wfID,
		QueryType:  "getState",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
}

func TestE2E_QueryWorkflow_ErrorResponse(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	// Start workflow
	startResp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
		WorkflowType: "QueryErrorWorkflow",
		TaskQueue:    "query-err-queue",
		Input:        []byte(`{}`),
	})
	require.NoError(t, err)
	wfID := startResp.GetWorkflowId()

	// Poll initial WT and complete with activity (to keep WF running)
	wt := pollWorkflowTaskUntil(t, ctx, "query-err-queue", "worker-1", 5*time.Second)
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt.GetTaskId(),
		Commands: []*apiv1.Command{
			scheduleActivityCmd(1, "doWork", json.RawMessage(`{}`), 30*time.Second, nil),
		},
	})
	require.NoError(t, err)

	// Send query in a goroutine
	type queryResult struct {
		resp *apiv1.QueryWorkflowResponse
		err  error
	}
	ch := make(chan queryResult, 1)
	go func() {
		resp, err := client.QueryWorkflow(ctx, &apiv1.QueryWorkflowRequest{
			WorkflowId: wfID,
			QueryType:  "badQuery",
		})
		ch <- queryResult{resp, err}
	}()

	// Poll WT triggered by query
	wt2 := pollWorkflowTaskUntil(t, ctx, "query-err-queue", "worker-1", 5*time.Second)
	require.NotEmpty(t, wt2.GetPendingQueries())

	pq := wt2.GetPendingQueries()[0]

	// Respond with error
	_, err = client.RespondQueryTask(ctx, &apiv1.RespondQueryTaskRequest{
		QueryId:      pq.GetQueryId(),
		ErrorMessage: "unknown query type: badQuery",
	})
	require.NoError(t, err)

	// Complete WT
	_, err = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
		TaskId: wt2.GetTaskId(),
	})
	require.NoError(t, err)

	// Wait for QueryWorkflow to return
	result := <-ch
	require.NoError(t, result.err)
	assert.Equal(t, "unknown query type: badQuery", result.resp.GetErrorMessage())
}
