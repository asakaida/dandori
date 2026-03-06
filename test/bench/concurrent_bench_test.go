package bench_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/google/uuid"
)

// BenchmarkConcurrentTaskThroughput measures task throughput with N concurrent workers.
func BenchmarkConcurrentTaskThroughput(b *testing.B) {
	for _, numWorkers := range []int{1, 4, 8, 16} {
		b.Run(fmt.Sprintf("workers=%d", numWorkers), func(b *testing.B) {
			truncateAll(b)
			ctx := context.Background()
			actTasks := store.ActivityTasks()

			// Pre-create workflows and enqueue tasks
			wfIDs := make([]uuid.UUID, b.N)
			for i := 0; i < b.N; i++ {
				wfIDs[i] = uuid.New()
				if err := store.Workflows().Create(ctx, domain.WorkflowExecution{
					ID: wfIDs[i], Namespace: "default", WorkflowType: "bench", TaskQueue: "bench-queue", Status: domain.WorkflowStatusRunning,
				}); err != nil {
					b.Fatalf("Create workflow: %v", err)
				}

				if err := actTasks.Enqueue(ctx, domain.ActivityTask{
					Namespace:     "default",
					QueueName:     "bench-queue",
					WorkflowID:    wfIDs[i],
					ActivityType:  "bench-activity",
					ActivityInput: json.RawMessage(`{}`),
					ActivitySeqID: int64(i),
					Attempt:       1,
					MaxAttempts:   3,
					ScheduledAt:   time.Now(),
				}); err != nil {
					b.Fatalf("Enqueue: %v", err)
				}
			}

			b.ResetTimer()

			var completed atomic.Int64
			var wg sync.WaitGroup
			wg.Add(numWorkers)

			for w := 0; w < numWorkers; w++ {
				go func(workerID int) {
					defer wg.Done()
					workerName := fmt.Sprintf("worker-%d", workerID)

					for {
						if completed.Load() >= int64(b.N) {
							return
						}

						task, err := actTasks.Poll(ctx, "default", "bench-queue", workerName)
						if err != nil {
							if err == domain.ErrNoTaskAvailable {
								if completed.Load() >= int64(b.N) {
									return
								}
								time.Sleep(time.Millisecond)
								continue
							}
							b.Errorf("Poll: %v", err)
							return
						}

						if err := actTasks.Complete(ctx, task.ID); err != nil {
							b.Errorf("Complete: %v", err)
							return
						}
						completed.Add(1)
					}
				}(w)
			}

			wg.Wait()
		})
	}
}

// BenchmarkConcurrentWorkflowCreate measures concurrent workflow creation.
func BenchmarkConcurrentWorkflowCreate(b *testing.B) {
	for _, numWorkers := range []int{1, 4, 8} {
		b.Run(fmt.Sprintf("workers=%d", numWorkers), func(b *testing.B) {
			truncateAll(b)
			ctx := context.Background()

			b.ResetTimer()

			var idx atomic.Int64
			var wg sync.WaitGroup
			wg.Add(numWorkers)

			for w := 0; w < numWorkers; w++ {
				go func() {
					defer wg.Done()
					for {
						i := idx.Add(1) - 1
						if i >= int64(b.N) {
							return
						}
						wfID := uuid.New()
						if err := store.Workflows().Create(ctx, domain.WorkflowExecution{
							ID:           wfID,
							Namespace:    "default",
							WorkflowType: "bench-concurrent",
							TaskQueue:    "bench-queue",
							Status:       domain.WorkflowStatusRunning,
							Input:        json.RawMessage(`{"i":` + fmt.Sprintf("%d", i) + `}`),
						}); err != nil {
							b.Errorf("Create: %v", err)
							return
						}
					}
				}()
			}

			wg.Wait()
		})
	}
}

// BenchmarkConcurrentEventAppend measures concurrent event appending across different workflows.
func BenchmarkConcurrentEventAppend(b *testing.B) {
	for _, numWorkers := range []int{1, 4, 8} {
		b.Run(fmt.Sprintf("workers=%d", numWorkers), func(b *testing.B) {
			truncateAll(b)
			ctx := context.Background()
			events := store.Events()

			// Pre-create workflows (one per worker to avoid contention on same workflow)
			wfIDs := make([]uuid.UUID, numWorkers)
			for w := 0; w < numWorkers; w++ {
				wfIDs[w] = uuid.New()
				if err := store.Workflows().Create(ctx, domain.WorkflowExecution{
					ID: wfIDs[w], Namespace: "default", WorkflowType: "bench", TaskQueue: "bench-queue", Status: domain.WorkflowStatusRunning,
				}); err != nil {
					b.Fatalf("Create workflow: %v", err)
				}
			}

			b.ResetTimer()

			var idx atomic.Int64
			var wg sync.WaitGroup
			wg.Add(numWorkers)

			for w := 0; w < numWorkers; w++ {
				go func(workerIdx int) {
					defer wg.Done()
					wfID := wfIDs[workerIdx]
					for {
						i := idx.Add(1) - 1
						if i >= int64(b.N) {
							return
						}
						if err := events.Append(ctx, []domain.HistoryEvent{
							{
								WorkflowID: wfID,
								Type:       domain.EventActivityTaskScheduled,
								Data:       json.RawMessage(`{"i":` + fmt.Sprintf("%d", i) + `}`),
							},
						}); err != nil {
							b.Errorf("Append: %v", err)
							return
						}
					}
				}(w)
			}

			wg.Wait()
		})
	}
}
