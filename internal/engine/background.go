package engine

import (
	"context"
	"encoding/json"
	"log/slog"
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
				slog.Error("activity timeout check error", "error", err)
			}
		}
	}
}

func (w *BackgroundWorker) checkActivityTimeouts(ctx context.Context) error {
	// StartToClose timeouts (RUNNING tasks)
	tasks, err := w.activityTasks.GetTimedOut(ctx)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if err := w.handleTimedOutTask(ctx, task); err != nil {
			slog.Error("handle timed out task error", "task_id", task.ID, "error", err)
		}
	}

	// ScheduleToClose timeouts (PENDING or RUNNING tasks)
	schedCloseTasks, err := w.activityTasks.GetScheduleToCloseTimedOut(ctx)
	if err != nil {
		return err
	}
	for _, task := range schedCloseTasks {
		if task.Status == domain.TaskStatusPending {
			if err := w.handleScheduleToStartTimedOutTask(ctx, task); err != nil {
				slog.Error("handle schedule-to-close timed out pending task error", "task_id", task.ID, "error", err)
			}
		} else {
			if err := w.handleTimedOutTask(ctx, task); err != nil {
				slog.Error("handle schedule-to-close timed out task error", "task_id", task.ID, "error", err)
			}
		}
	}

	// ScheduleToStart timeouts (PENDING tasks only)
	schedStartTasks, err := w.activityTasks.GetScheduleToStartTimedOut(ctx)
	if err != nil {
		return err
	}
	for _, task := range schedStartTasks {
		if err := w.handleScheduleToStartTimedOutTask(ctx, task); err != nil {
			slog.Error("handle schedule-to-start timed out task error", "task_id", task.ID, "error", err)
		}
	}

	return nil
}

func (w *BackgroundWorker) handleTimedOutTask(ctx context.Context, task domain.ActivityTask) error {
	return w.tx.RunInTx(ctx, func(ctx context.Context) error {
		wf, err := w.workflows.Get(ctx, task.Namespace, task.WorkflowID)
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
			Namespace:   task.Namespace,
			QueueName:   wf.TaskQueue,
			WorkflowID:  task.WorkflowID,
			ScheduledAt: time.Now(),
		})
	})
}

func (w *BackgroundWorker) handleScheduleToStartTimedOutTask(ctx context.Context, task domain.ActivityTask) error {
	return w.tx.RunInTx(ctx, func(ctx context.Context) error {
		wf, err := w.workflows.Get(ctx, task.Namespace, task.WorkflowID)
		if err != nil {
			return err
		}

		if err := w.activityTasks.CompletePending(ctx, task.ID); err != nil {
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
			Namespace:   task.Namespace,
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
				slog.Error("heartbeat timeout check error", "error", err)
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
			slog.Error("handle heartbeat timed out task error", "task_id", task.ID, "error", err)
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
				slog.Error("timer poller error", "error", err)
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
			slog.Error("handle fired timer error", "timer_id", timer.ID, "error", err)
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

		wf, err := w.workflows.Get(ctx, timer.Namespace, timer.WorkflowID)
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
			Namespace:   timer.Namespace,
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
				slog.Error("workflow task recovery error", "error", err)
			} else if n > 0 {
				slog.Info("recovered stale workflow tasks", "count", n)
			}

			if n, err := w.activityTasks.RecoverStaleTasks(ctx); err != nil {
				slog.Error("activity task recovery error", "error", err)
			} else if n > 0 {
				slog.Info("recovered stale activity tasks", "count", n)
			}
		}
	}
}
