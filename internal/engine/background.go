package engine

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/asakaida/dandori/internal/domain"
	"github.com/asakaida/dandori/internal/port"
)

type BackgroundWorker struct {
	workflows     port.WorkflowRepository
	events        port.EventRepository
	workflowTasks port.WorkflowTaskRepository
	activityTasks port.ActivityTaskRepository
	timers        port.TimerRepository
	tx            port.TxManager
}

func NewBackgroundWorker(
	workflows port.WorkflowRepository,
	events port.EventRepository,
	workflowTasks port.WorkflowTaskRepository,
	activityTasks port.ActivityTaskRepository,
	timers port.TimerRepository,
	tx port.TxManager,
) *BackgroundWorker {
	return &BackgroundWorker{
		workflows:     workflows,
		events:        events,
		workflowTasks: workflowTasks,
		activityTasks: activityTasks,
		timers:        timers,
		tx:            tx,
	}
}

func (w *BackgroundWorker) RunActivityTimeoutChecker(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.checkActivityTimeouts(ctx); err != nil {
				log.Printf("activity timeout check error: %v", err)
			}
		}
	}
}

func (w *BackgroundWorker) checkActivityTimeouts(ctx context.Context) error {
	tasks, err := w.activityTasks.GetTimedOut(ctx)
	if err != nil {
		return err
	}

	for _, task := range tasks {
		if err := w.handleTimedOutTask(ctx, task); err != nil {
			log.Printf("handle timed out task %d error: %v", task.ID, err)
		}
	}
	return nil
}

func (w *BackgroundWorker) handleTimedOutTask(ctx context.Context, task domain.ActivityTask) error {
	return w.tx.RunInTx(ctx, func(ctx context.Context) error {
		wf, err := w.workflows.Get(ctx, task.WorkflowID)
		if err != nil {
			return err
		}

		if err := w.activityTasks.Complete(ctx, task.ID); err != nil {
			return err
		}

		if wf.Status.IsTerminal() {
			return nil
		}

		eventData, err := json.Marshal(map[string]any{
			"activity_seq_id": task.ActivitySeqID,
		})
		if err != nil {
			return err
		}
		if err := w.events.Append(ctx, []domain.HistoryEvent{
			{WorkflowID: task.WorkflowID, Type: domain.EventActivityTaskTimedOut, Data: eventData},
		}); err != nil {
			return err
		}

		return w.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
			QueueName:   wf.TaskQueue,
			WorkflowID:  task.WorkflowID,
			ScheduledAt: time.Now(),
		})
	})
}

func (w *BackgroundWorker) RunHeartbeatTimeoutChecker(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.checkHeartbeatTimeouts(ctx); err != nil {
				log.Printf("heartbeat timeout check error: %v", err)
			}
		}
	}
}

func (w *BackgroundWorker) checkHeartbeatTimeouts(ctx context.Context) error {
	tasks, err := w.activityTasks.GetHeartbeatTimedOut(ctx)
	if err != nil {
		return err
	}

	for _, task := range tasks {
		if err := w.handleTimedOutTask(ctx, task); err != nil {
			log.Printf("handle heartbeat timed out task %d error: %v", task.ID, err)
		}
	}
	return nil
}

func (w *BackgroundWorker) RunTimerPoller(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.pollFiredTimers(ctx); err != nil {
				log.Printf("timer poller error: %v", err)
			}
		}
	}
}

func (w *BackgroundWorker) pollFiredTimers(ctx context.Context) error {
	timers, err := w.timers.GetFired(ctx)
	if err != nil {
		return err
	}

	for _, timer := range timers {
		if err := w.handleFiredTimer(ctx, timer); err != nil {
			log.Printf("handle fired timer %d error: %v", timer.ID, err)
		}
	}
	return nil
}

func (w *BackgroundWorker) handleFiredTimer(ctx context.Context, timer domain.Timer) error {
	return w.tx.RunInTx(ctx, func(ctx context.Context) error {
		fired, err := w.timers.MarkFired(ctx, timer.ID)
		if err != nil {
			return err
		}
		if !fired {
			return nil
		}

		wf, err := w.workflows.Get(ctx, timer.WorkflowID)
		if err != nil {
			return err
		}
		if wf.Status.IsTerminal() {
			return nil
		}

		eventData, err := json.Marshal(map[string]any{
			"seq_id": timer.SeqID,
		})
		if err != nil {
			return err
		}
		if err := w.events.Append(ctx, []domain.HistoryEvent{
			{WorkflowID: timer.WorkflowID, Type: domain.EventTimerFired, Data: eventData},
		}); err != nil {
			return err
		}

		return w.workflowTasks.Enqueue(ctx, domain.WorkflowTask{
			QueueName:   wf.TaskQueue,
			WorkflowID:  timer.WorkflowID,
			ScheduledAt: time.Now(),
		})
	})
}

func (w *BackgroundWorker) RunTaskRecovery(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if n, err := w.workflowTasks.RecoverStaleTasks(ctx); err != nil {
				log.Printf("workflow task recovery error: %v", err)
			} else if n > 0 {
				log.Printf("recovered %d stale workflow tasks", n)
			}

			if n, err := w.activityTasks.RecoverStaleTasks(ctx); err != nil {
				log.Printf("activity task recovery error: %v", err)
			} else if n > 0 {
				log.Printf("recovered %d stale activity tasks", n)
			}
		}
	}
}
