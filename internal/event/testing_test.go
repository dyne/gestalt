package event

import (
	"context"
	"testing"
	"time"
)

func TestEventCollectorCollectsEvents(t *testing.T) {
	collector := NewEventCollector[int]()
	collector.Collect(1)
	collector.Collect(2)

	events := collector.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0] != 1 || events[1] != 2 {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestMockBusPublishesAndFilters(t *testing.T) {
	bus := NewMockBus[int]()
	defer bus.Close()

	events, cancel := bus.SubscribeFiltered(func(value int) bool {
		return value%2 == 0
	})
	defer cancel()

	bus.Publish(1)
	bus.Publish(2)

	got := ReceiveWithTimeout(t, events, 100*time.Millisecond)
	if got != 2 {
		t.Fatalf("expected event 2, got %d", got)
	}
	busEvents := bus.Events()
	if len(busEvents) != 2 {
		t.Fatalf("expected 2 stored events, got %d", len(busEvents))
	}
}

func TestReceiveWithTimeoutReceivesBusEvent(t *testing.T) {
	bus := NewBus[string](context.Background(), BusOptions{})
	defer bus.Close()

	events, cancel := bus.Subscribe()
	defer cancel()

	bus.Publish("ok")
	received := ReceiveWithTimeout(t, events, 100*time.Millisecond)
	if received != "ok" {
		t.Fatalf("expected ok, got %q", received)
	}
}

func TestEventMatcherAppliesPredicates(t *testing.T) {
	MatchEvent(t, "alpha").
		Require("expected alpha", func(value string) bool {
			return value == "alpha"
		})
}
