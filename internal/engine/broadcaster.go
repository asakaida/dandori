package engine

import (
	"sync"
)

// WorkflowNotification is sent when a workflow's state changes.
type WorkflowNotification struct {
	WorkflowID string `json:"workflowId"`
	Namespace  string `json:"namespace"`
}

type sseSubscriber struct {
	ch        chan WorkflowNotification
	namespace string // empty = all namespaces
}

// Broadcaster distributes workflow notifications to SSE subscribers.
type Broadcaster struct {
	mu          sync.RWMutex
	subscribers map[*sseSubscriber]struct{}
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[*sseSubscriber]struct{}),
	}
}

// Subscribe returns a channel that receives notifications.
// If namespace is non-empty, only events for that namespace are sent.
// The returned function must be called to unsubscribe.
func (b *Broadcaster) Subscribe(namespace string) (<-chan WorkflowNotification, func()) {
	sub := &sseSubscriber{
		ch:        make(chan WorkflowNotification, 64),
		namespace: namespace,
	}

	b.mu.Lock()
	b.subscribers[sub] = struct{}{}
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		delete(b.subscribers, sub)
		b.mu.Unlock()
		close(sub.ch)
	}

	return sub.ch, unsubscribe
}

// Publish sends a notification to all matching subscribers.
func (b *Broadcaster) Publish(event WorkflowNotification) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for sub := range b.subscribers {
		if sub.namespace != "" && sub.namespace != event.Namespace {
			continue
		}
		select {
		case sub.ch <- event:
		default:
			// subscriber too slow, drop event
		}
	}
}
