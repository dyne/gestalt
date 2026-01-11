package event

import (
	"sync"
	"testing"
	"time"
)

const defaultMockBusBufferSize = 16

// MockBus captures published events and synchronously fan-outs to subscribers.
type MockBus[T any] struct {
	mu          sync.Mutex
	subscribers map[uint64]mockSubscription[T]
	nextID      uint64
	bufferSize  int
	events      []T
}

type mockSubscription[T any] struct {
	ch     chan T
	filter func(T) bool
}

func NewMockBus[T any]() *MockBus[T] {
	return NewMockBusWithBuffer[T](defaultMockBusBufferSize)
}

func NewMockBusWithBuffer[T any](bufferSize int) *MockBus[T] {
	if bufferSize <= 0 {
		bufferSize = defaultMockBusBufferSize
	}
	return &MockBus[T]{
		subscribers: make(map[uint64]mockSubscription[T]),
		bufferSize:  bufferSize,
	}
}

func (bus *MockBus[T]) Publish(event T) {
	if bus == nil {
		return
	}
	bus.mu.Lock()
	bus.events = append(bus.events, event)
	subscribers := make([]mockSubscription[T], 0, len(bus.subscribers))
	for _, sub := range bus.subscribers {
		subscribers = append(subscribers, sub)
	}
	bus.mu.Unlock()

	for _, sub := range subscribers {
		if sub.filter != nil && !sub.filter(event) {
			continue
		}
		select {
		case sub.ch <- event:
		default:
		}
	}
}

func (bus *MockBus[T]) Subscribe() (<-chan T, func()) {
	return bus.SubscribeFiltered(nil)
}

func (bus *MockBus[T]) SubscribeFiltered(filter func(T) bool) (<-chan T, func()) {
	if bus == nil {
		ch := make(chan T)
		close(ch)
		return ch, func() {}
	}
	ch := make(chan T, bus.bufferSize)
	bus.mu.Lock()
	bus.nextID++
	id := bus.nextID
	bus.subscribers[id] = mockSubscription[T]{ch: ch, filter: filter}
	bus.mu.Unlock()

	cancel := func() {
		bus.mu.Lock()
		sub, ok := bus.subscribers[id]
		delete(bus.subscribers, id)
		bus.mu.Unlock()
		if ok {
			close(sub.ch)
		}
	}

	return ch, cancel
}

func (bus *MockBus[T]) Close() {
	if bus == nil {
		return
	}
	bus.mu.Lock()
	subscribers := bus.subscribers
	bus.subscribers = make(map[uint64]mockSubscription[T])
	bus.mu.Unlock()

	for _, sub := range subscribers {
		close(sub.ch)
	}
}

func (bus *MockBus[T]) Events() []T {
	if bus == nil {
		return nil
	}
	bus.mu.Lock()
	defer bus.mu.Unlock()
	copyEvents := make([]T, len(bus.events))
	copy(copyEvents, bus.events)
	return copyEvents
}

// EventCollector stores events received from callbacks or subscriptions.
type EventCollector[T any] struct {
	mu     sync.Mutex
	events []T
}

func NewEventCollector[T any]() *EventCollector[T] {
	return &EventCollector[T]{}
}

func (collector *EventCollector[T]) Collect(event T) {
	if collector == nil {
		return
	}
	collector.mu.Lock()
	collector.events = append(collector.events, event)
	collector.mu.Unlock()
}

func (collector *EventCollector[T]) Events() []T {
	if collector == nil {
		return nil
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	copyEvents := make([]T, len(collector.events))
	copy(copyEvents, collector.events)
	return copyEvents
}

// ReceiveWithTimeout waits for a single event or fails the test.
func ReceiveWithTimeout[T any](t *testing.T, ch <-chan T, timeout time.Duration) T {
	t.Helper()
	select {
	case event, ok := <-ch:
		if !ok {
			t.Fatal("event channel closed")
		}
		return event
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for event after %s", timeout)
	}
	var zero T
	return zero
}

// EventMatcher provides fluent assertions over event properties.
type EventMatcher[T any] struct {
	testing *testing.T
	event   T
}

func MatchEvent[T any](t *testing.T, event T) *EventMatcher[T] {
	if t != nil {
		t.Helper()
	}
	return &EventMatcher[T]{testing: t, event: event}
}

func (matcher *EventMatcher[T]) Require(message string, predicate func(T) bool) *EventMatcher[T] {
	if matcher == nil || matcher.testing == nil {
		return matcher
	}
	matcher.testing.Helper()
	if !predicate(matcher.event) {
		matcher.testing.Fatalf("%s", message)
	}
	return matcher
}

func (matcher *EventMatcher[T]) Event() T {
	if matcher == nil {
		var zero T
		return zero
	}
	return matcher.event
}
