package events

import (
	"sync"

	"github.com/jlaneve/cwt-cli/internal/types"
)

// Bus provides a simple event bus for publishing and subscribing to events
type Bus struct {
	subscribers []chan types.Event
	mu          sync.RWMutex
}

// NewBus creates a new event bus
func NewBus() *Bus {
	return &Bus{
		subscribers: make([]chan types.Event, 0),
	}
}

// Subscribe returns a channel that will receive all published events
func (b *Bus) Subscribe() <-chan types.Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan types.Event, 100) // Buffered to prevent blocking
	b.subscribers = append(b.subscribers, ch)
	return ch
}

// Publish sends an event to all subscribers
func (b *Bus) Publish(event types.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers {
		select {
		case ch <- event:
			// Event sent successfully
		default:
			// Subscriber channel is full, skip to prevent blocking
			// In a production system, you might want to log this
		}
	}
}

// Close closes all subscriber channels
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ch := range b.subscribers {
		close(ch)
	}
	b.subscribers = nil
}

// SubscriberCount returns the number of active subscribers
func (b *Bus) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}
