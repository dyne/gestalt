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

type sampleEvent struct {
	kind string
}

func (s sampleEvent) Type() string {
	return s.kind
}
