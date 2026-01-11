package event

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"gestalt/internal/metrics"
)

func TestBusSubscribePublish(t *testing.T) {
	bus := NewBus[int](context.Background(), BusOptions{})
	t.Cleanup(bus.Close)

	ch, cancel := bus.Subscribe()
	defer cancel()

	bus.Publish(42)

	select {
	case got := <-ch:
		if got != 42 {
			t.Fatalf("expected 42, got %d", got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}

	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to close after cancel")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for channel close")
	}
}

func TestBusCloseClosesSubscribers(t *testing.T) {
	bus := NewBus[int](context.Background(), BusOptions{})
	ch, _ := bus.Subscribe()

	bus.Close()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to close after bus close")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for channel close")
	}
}

func TestBusDropOnFull(t *testing.T) {
	registry := &metrics.Registry{}
	bus := NewBus[string](context.Background(), BusOptions{
		Name:                 "drop",
		SubscriberBufferSize: 1,
		Registry:             registry,
	})
	t.Cleanup(bus.Close)

	ch, _ := bus.Subscribe()

	bus.Publish("first")

	done := make(chan struct{})
	go func() {
		bus.Publish("second")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("publish blocked in drop mode")
	}

	select {
	case got := <-ch:
		if got != "first" {
			t.Fatalf("expected first event, got %q", got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for first event")
	}

	select {
	case got := <-ch:
		t.Fatalf("unexpected event %q", got)
	case <-time.After(50 * time.Millisecond):
	}

	var output bytes.Buffer
	if err := registry.WritePrometheus(&output); err != nil {
		t.Fatalf("write metrics: %v", err)
	}
	body := output.String()
	if !strings.Contains(body, "gestalt_events_published_total{bus=\"drop\",type=\"unknown\"} 2") {
		t.Fatalf("expected published metrics, got %q", body)
	}
	if !strings.Contains(body, "gestalt_events_dropped_total{bus=\"drop\",type=\"unknown\"} 1") {
		t.Fatalf("expected dropped metrics, got %q", body)
	}
}

func TestBusHistoryStoresRecentEvents(t *testing.T) {
	bus := NewBus[int](context.Background(), BusOptions{
		HistorySize: 2,
	})
	t.Cleanup(bus.Close)

	bus.Publish(1)
	bus.Publish(2)
	bus.Publish(3)

	history := bus.DumpHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 history events, got %d", len(history))
	}
	if history[0] != 2 || history[1] != 3 {
		t.Fatalf("unexpected history events: %#v", history)
	}
}

func TestBusReplayLastSendsRecentEvents(t *testing.T) {
	bus := NewBus[int](context.Background(), BusOptions{
		HistorySize: 3,
	})
	t.Cleanup(bus.Close)

	bus.Publish(1)
	bus.Publish(2)
	bus.Publish(3)

	replay := make(chan int, 2)
	bus.ReplayLast(2, replay)

	first := ReceiveWithTimeout(t, replay, 100*time.Millisecond)
	second := ReceiveWithTimeout(t, replay, 100*time.Millisecond)
	if first != 2 || second != 3 {
		t.Fatalf("unexpected replay events: %d, %d", first, second)
	}
}

func TestBusBlockOnFullTimeout(t *testing.T) {
	bus := NewBus[int](context.Background(), BusOptions{
		Name:                 "block",
		SubscriberBufferSize: 1,
		BlockOnFull:          true,
		WriteTimeout:         20 * time.Millisecond,
	})
	t.Cleanup(bus.Close)

	ch, _ := bus.Subscribe()

	bus.Publish(1)

	done := make(chan struct{})
	go func() {
		bus.Publish(2)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("publish returned too early in block mode")
	case <-time.After(10 * time.Millisecond):
	}

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("publish did not return after timeout")
	}

	select {
	case got := <-ch:
		if got != 1 {
			t.Fatalf("expected first event, got %d", got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for first event")
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to close after timeout")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for channel close")
	}
}

func TestBusSubscribeFiltered(t *testing.T) {
	bus := NewBus[int](context.Background(), BusOptions{})
	t.Cleanup(bus.Close)

	ch, _ := bus.SubscribeFiltered(func(value int) bool {
		return value%2 == 0
	})

	bus.Publish(1)
	bus.Publish(2)

	select {
	case got := <-ch:
		if got != 2 {
			t.Fatalf("expected filtered event 2, got %d", got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for filtered event")
	}

	select {
	case got := <-ch:
		t.Fatalf("unexpected event %d", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBusSubscribeType(t *testing.T) {
	bus := NewBus[Event](context.Background(), BusOptions{})
	t.Cleanup(bus.Close)

	ch, _ := bus.SubscribeType("agent_started")

	bus.Publish(NewAgentEvent("agent-1", "Ada", "agent_started"))

	select {
	case event := <-ch:
		if event.Type() != "agent_started" {
			t.Fatalf("expected agent_started, got %q", event.Type())
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for typed event")
	}
}

func TestBusSubscribeTypes(t *testing.T) {
	bus := NewBus[Event](context.Background(), BusOptions{})
	t.Cleanup(bus.Close)

	ch, _ := bus.SubscribeTypes("agent_started", "agent_stopped")

	bus.Publish(NewAgentEvent("agent-1", "Ada", "agent_started"))
	bus.Publish(NewAgentEvent("agent-1", "Ada", "agent_stopped"))

	first := readEvent(t, ch)
	second := readEvent(t, ch)

	if first.Type() != "agent_started" {
		t.Fatalf("expected agent_started, got %q", first.Type())
	}
	if second.Type() != "agent_stopped" {
		t.Fatalf("expected agent_stopped, got %q", second.Type())
	}
}

func TestBusSubscriberMetrics(t *testing.T) {
	registry := &metrics.Registry{}
	bus := NewBus[int](context.Background(), BusOptions{
		Name:     "subs",
		Registry: registry,
	})
	t.Cleanup(bus.Close)

	_, cancelUnfiltered := bus.Subscribe()
	_, cancelFiltered := bus.SubscribeFiltered(func(value int) bool {
		return value > 0
	})
	defer cancelUnfiltered()
	defer cancelFiltered()

	var output bytes.Buffer
	if err := registry.WritePrometheus(&output); err != nil {
		t.Fatalf("write metrics: %v", err)
	}
	body := output.String()
	if !strings.Contains(body, "gestalt_event_subscribers{bus=\"subs\",filtered=\"true\"} 1") {
		t.Fatalf("expected filtered subscriber metric, got %q", body)
	}
	if !strings.Contains(body, "gestalt_event_subscribers{bus=\"subs\",filtered=\"false\"} 1") {
		t.Fatalf("expected unfiltered subscriber metric, got %q", body)
	}
}

func TestBusContextCancelCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	bus := NewBus[int](ctx, BusOptions{})

	ch, _ := bus.Subscribe()
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to close after context cancel")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for channel close")
	}
}

func TestBusMetricsEventType(t *testing.T) {
	registry := &metrics.Registry{}
	bus := NewBus[sampleEvent](context.Background(), BusOptions{
		Name:     "typed",
		Registry: registry,
	})
	t.Cleanup(bus.Close)

	bus.Publish(sampleEvent{kind: "alpha"})

	var output bytes.Buffer
	if err := registry.WritePrometheus(&output); err != nil {
		t.Fatalf("write metrics: %v", err)
	}
	body := output.String()
	if !strings.Contains(body, "gestalt_events_published_total{bus=\"typed\",type=\"alpha\"} 1") {
		t.Fatalf("expected typed metrics, got %q", body)
	}
}

func TestBusConcurrentSubscribePublish(t *testing.T) {
	bus := NewBus[int](context.Background(), BusOptions{})
	t.Cleanup(bus.Close)

	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(value int) {
			defer wg.Done()
			ch, cancel := bus.Subscribe()
			defer cancel()
			bus.Publish(value)
			select {
			case <-ch:
			case <-time.After(100 * time.Millisecond):
				t.Errorf("timeout waiting for event %d", value)
			}
		}(i)
	}
	wg.Wait()
}

func TestBusNilEventIgnored(t *testing.T) {
	bus := NewBus[*int](context.Background(), BusOptions{})
	t.Cleanup(bus.Close)

	ch, _ := bus.Subscribe()
	bus.Publish((*int)(nil))

	select {
	case <-ch:
		t.Fatal("expected nil event to be ignored")
	case <-time.After(50 * time.Millisecond):
	}
}

func readEvent(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case event := <-ch:
		return event
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
		return nil
	}
}

type sampleEvent struct {
	kind string
}

func (s sampleEvent) Type() string {
	return s.kind
}
