package telemetry

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	WorkflowStartedTotal   prometheus.Counter
	WorkflowCompletedTotal prometheus.Counter
	WorkflowFailedTotal    prometheus.Counter
	WorkflowTerminatedTotal prometheus.Counter
	WorkflowCanceledTotal  prometheus.Counter

	WorkflowTaskPollTotal     prometheus.Counter
	WorkflowTaskCompleteTotal prometheus.Counter
	WorkflowTaskFailTotal     prometheus.Counter

	ActivityTaskPollTotal     prometheus.Counter
	ActivityTaskCompleteTotal prometheus.Counter
	ActivityTaskFailTotal     prometheus.Counter

	OperationDuration *prometheus.HistogramVec

	ActiveWorkflows prometheus.Gauge
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		WorkflowStartedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dandori_workflow_started_total",
			Help: "Total number of workflows started.",
		}),
		WorkflowCompletedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dandori_workflow_completed_total",
			Help: "Total number of workflows completed (via DescribeWorkflow observation).",
		}),
		WorkflowFailedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dandori_workflow_failed_total",
			Help: "Total number of workflow task failures.",
		}),
		WorkflowTerminatedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dandori_workflow_terminated_total",
			Help: "Total number of workflows terminated.",
		}),
		WorkflowCanceledTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dandori_workflow_canceled_total",
			Help: "Total number of workflows canceled.",
		}),
		WorkflowTaskPollTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dandori_workflow_task_poll_total",
			Help: "Total number of workflow task polls.",
		}),
		WorkflowTaskCompleteTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dandori_workflow_task_complete_total",
			Help: "Total number of workflow task completions.",
		}),
		WorkflowTaskFailTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dandori_workflow_task_fail_total",
			Help: "Total number of workflow task failures.",
		}),
		ActivityTaskPollTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dandori_activity_task_poll_total",
			Help: "Total number of activity task polls.",
		}),
		ActivityTaskCompleteTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dandori_activity_task_complete_total",
			Help: "Total number of activity task completions.",
		}),
		ActivityTaskFailTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dandori_activity_task_fail_total",
			Help: "Total number of activity task failures.",
		}),
		OperationDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "dandori_operation_duration_seconds",
			Help:    "Duration of service operations in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation"}),
		ActiveWorkflows: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "dandori_active_workflows",
			Help: "Number of currently running workflows (approximate).",
		}),
	}

	reg.MustRegister(
		m.WorkflowStartedTotal,
		m.WorkflowCompletedTotal,
		m.WorkflowFailedTotal,
		m.WorkflowTerminatedTotal,
		m.WorkflowCanceledTotal,
		m.WorkflowTaskPollTotal,
		m.WorkflowTaskCompleteTotal,
		m.WorkflowTaskFailTotal,
		m.ActivityTaskPollTotal,
		m.ActivityTaskCompleteTotal,
		m.ActivityTaskFailTotal,
		m.OperationDuration,
		m.ActiveWorkflows,
	)

	return m
}
