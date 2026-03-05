package e2e_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_MultipleWorkersNoDuplicate(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	const numWorkflows = 5
	const numWorkers = 3
	queue := "concurrent-queue"

	// Start 5 workflows
	wfIDs := make([]string, numWorkflows)
	for i := 0; i < numWorkflows; i++ {
		resp, err := client.StartWorkflow(ctx, &apiv1.StartWorkflowRequest{
			WorkflowType: "ConcurrentWorkflow",
			TaskQueue:    queue,
			Input:        []byte(`{}`),
		})
		require.NoError(t, err)
		wfIDs[i] = resp.GetWorkflowId()
	}

	// Small delay so all tasks are pollable
	time.Sleep(100 * time.Millisecond)

	// 3 workers poll concurrently, collecting task -> workflow_id mappings
	var mu sync.Mutex
	polled := make(map[string]string) // workflow_id -> worker_id
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		workerID := "worker-" + string(rune('1'+w))
		go func(wid string) {
			defer wg.Done()
			for {
				resp, err := client.PollWorkflowTask(ctx, &apiv1.PollWorkflowTaskRequest{
					QueueName: queue,
					WorkerId:  wid,
				})
				if err != nil {
					return
				}
				if resp.GetTaskId() == 0 {
					return // no more tasks
				}
				mu.Lock()
				polled[resp.GetWorkflowId()] = wid
				mu.Unlock()

				// Complete the workflow task immediately
				result, _ := json.Marshal(map[string]string{"worker": wid})
				attrs, _ := json.Marshal(map[string]json.RawMessage{"result": result})
				_, _ = client.CompleteWorkflowTask(ctx, &apiv1.CompleteWorkflowTaskRequest{
					TaskId: resp.GetTaskId(),
					Commands: []*apiv1.Command{
						{Type: apiv1.CommandType_COMMAND_TYPE_COMPLETE_WORKFLOW, Attributes: attrs},
					},
				})
			}
		}(workerID)
	}

	wg.Wait()

	// Each workflow should have been polled exactly once (no duplicates)
	assert.Equal(t, numWorkflows, len(polled), "each workflow should be polled exactly once, got %d", len(polled))

	// Verify all workflows are COMPLETED
	for _, wfID := range wfIDs {
		desc, err := client.DescribeWorkflow(ctx, &apiv1.DescribeWorkflowRequest{WorkflowId: wfID})
		require.NoError(t, err)
		assert.Equal(t, apiv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED, desc.GetWorkflowExecution().GetStatus(),
			"workflow %s should be COMPLETED", wfID)
	}
}
