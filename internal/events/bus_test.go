package events

import (
	"sync"
	"testing"
	"time"

	"github.com/jlaneve/cwt-cli/internal/types"
)

func TestEventBus_Subscribe(t *testing.T) {
	bus := NewBus()

	ch := bus.Subscribe()
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}

	// Test multiple subscriptions
	ch2 := bus.Subscribe()
	if ch2 == ch {
		t.Error("Subscribe returned same channel for different subscriptions")
	}

	// Verify subscriber count
	if bus.SubscriberCount() != 2 {
		t.Errorf("SubscriberCount() = %d, want 2", bus.SubscriberCount())
	}
}

func TestEventBus_Publish(t *testing.T) {
	bus := NewBus()

	// Subscribe to events
	ch := bus.Subscribe()

	// Publish event
	event := types.SessionCreated{
		Session: types.Session{
			Core: types.CoreSession{
				ID:   "test-session",
				Name: "test",
			},
		},
	}

	go func() {
		bus.Publish(event)
	}()

	// Wait for event
	select {
	case received := <-ch:
		if received.EventType() != event.EventType() {
			t.Errorf("Received event type %v, want %v", received.EventType(), event.EventType())
		}
		if created, ok := received.(types.SessionCreated); ok {
			if created.Session.Core.ID != event.Session.Core.ID {
				t.Errorf("Received session ID %v, want %v", created.Session.Core.ID, event.Session.Core.ID)
			}
		} else {
			t.Error("Failed to cast event to SessionCreated")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for event")
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewBus()

	// Create multiple subscribers
	var wg sync.WaitGroup
	receivedCount := 0
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		ch := bus.Subscribe()
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ch:
				mu.Lock()
				receivedCount++
				mu.Unlock()
			case <-time.After(100 * time.Millisecond):
			}
		}()
	}

	// Publish event
	event := types.SessionDeleted{
		SessionID: "test-session",
	}
	bus.Publish(event)

	// Wait for all subscribers
	wg.Wait()

	if receivedCount != 3 {
		t.Errorf("Received count = %d, want 3", receivedCount)
	}
}

func TestEventBus_BufferOverflow(t *testing.T) {
	bus := NewBus()
	ch := bus.Subscribe()

	// Fill the buffer
	for i := 0; i < 101; i++ {
		event := types.SessionCreated{
			Session: types.Session{
				Core: types.CoreSession{
					ID:   "test",
					Name: "test",
				},
			},
		}
		bus.Publish(event)
	}

	// Count received events (should be at most 100 due to buffer size)
	count := 0
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-ch:
				count++
			case <-time.After(50 * time.Millisecond):
				done <- true
				return
			}
		}
	}()

	<-done
	if count != 100 {
		t.Errorf("Expected 100 events in buffer, got %d", count)
	}
}

func TestEventBus_Close(t *testing.T) {
	bus := NewBus()
	ch1 := bus.Subscribe()
	ch2 := bus.Subscribe()

	// Close the bus
	bus.Close()

	// Verify channels are closed
	_, ok1 := <-ch1
	if ok1 {
		t.Error("Channel 1 should be closed")
	}

	_, ok2 := <-ch2
	if ok2 {
		t.Error("Channel 2 should be closed")
	}

	// Verify subscriber count is 0
	if bus.SubscriberCount() != 0 {
		t.Errorf("SubscriberCount() = %d after Close(), want 0", bus.SubscriberCount())
	}
}

func TestEventBus_DifferentEventTypes(t *testing.T) {
	bus := NewBus()

	// Subscribe to events
	ch := bus.Subscribe()

	// Test different event types
	events := []types.Event{
		types.SessionCreationStarted{Name: "test"},
		types.SessionCreated{Session: types.Session{Core: types.CoreSession{ID: "1", Name: "test"}}},
		types.SessionCreationFailed{Name: "test", Error: "error"},
		types.SessionDeleted{SessionID: "1"},
		types.SessionDeletionFailed{SessionID: "1", Error: "error"},
		types.ClaudeStatusChanged{
			SessionID: "1",
			OldStatus: types.ClaudeStatus{State: types.ClaudeUnknown},
			NewStatus: types.ClaudeStatus{State: types.ClaudeWorking},
		},
		types.TmuxSessionDied{SessionID: "1", TmuxSession: "cwt-test"},
		types.GitChangesDetected{
			SessionID: "1",
			NewStatus: types.GitStatus{HasChanges: true, ModifiedFiles: []string{"test.txt"}},
		},
		types.RefreshCompleted{Sessions: []types.Session{}, Error: ""},
	}

	// Publish all events
	for _, event := range events {
		bus.Publish(event)
	}

	// Verify all events are received
	receivedTypes := make(map[string]bool)
	timeout := time.After(100 * time.Millisecond)

	for i := 0; i < len(events); i++ {
		select {
		case event := <-ch:
			receivedTypes[event.EventType()] = true
		case <-timeout:
			t.Fatalf("Timeout after receiving %d events", i)
		}
	}

	// Verify we received all event types
	for _, event := range events {
		if !receivedTypes[event.EventType()] {
			t.Errorf("Did not receive event type: %s", event.EventType())
		}
	}
}
