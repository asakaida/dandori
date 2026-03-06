package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type EventType string

const (
	EventWorkflowExecutionStarted    EventType = "WorkflowExecutionStarted"
	EventWorkflowExecutionCompleted  EventType = "WorkflowExecutionCompleted"
	EventWorkflowExecutionFailed     EventType = "WorkflowExecutionFailed"
	EventWorkflowExecutionTerminated EventType = "WorkflowExecutionTerminated"
	EventActivityTaskScheduled       EventType = "ActivityTaskScheduled"
	EventActivityTaskCompleted       EventType = "ActivityTaskCompleted"
	EventActivityTaskFailed          EventType = "ActivityTaskFailed"
	EventActivityTaskTimedOut        EventType = "ActivityTaskTimedOut"
	EventTimerStarted               EventType = "TimerStarted"
	EventTimerFired                 EventType = "TimerFired"
	EventTimerCanceled              EventType = "TimerCanceled"
	EventWorkflowSignaled           EventType = "WorkflowSignaled"
	EventWorkflowCancelRequested    EventType = "WorkflowCancelRequested"
)

type HistoryEvent struct {
	ID          int64
	WorkflowID  uuid.UUID
	SequenceNum int
	Type        EventType
	Data        json.RawMessage
	Timestamp   time.Time
}
