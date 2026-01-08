package watcher

import (
	"context"
	"testing"
	"time"
)

func TestEventHubSubscribePublish(t *testing.T) {
	hub := NewEventHub(context.Background(), nil)
	defer hub.Close()

	events := make(chan Event, 1)
	id := hub.Subscribe(EventTypeFileChanged, func(event Event) {
		events <- event
	})
	if id == "" {
		t.Fatal("expected subscription id")
	}

	hub.Publish(Event{
		Type: EventTypeFileChanged,
		Path: "PLAN.org",
	})

	select {
	case event := <-events:
		if event.Path != "PLAN.org" {
			t.Fatalf("expected path PLAN.org, got %q", event.Path)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestEventHubUnsubscribe(t *testing.T) {
	hub := NewEventHub(context.Background(), nil)
	defer hub.Close()

	events := make(chan Event, 1)
	id := hub.Subscribe(EventTypeFileChanged, func(event Event) {
		events <- event
	})
	hub.Unsubscribe(id)

	hub.Publish(Event{
		Type: EventTypeFileChanged,
		Path: "PLAN.org",
	})

	select {
	case <-events:
		t.Fatal("unexpected event after unsubscribe")
	case <-time.After(200 * time.Millisecond):
	}
}
