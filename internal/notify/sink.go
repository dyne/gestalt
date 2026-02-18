package notify

import (
	"context"
	"sync"
	"time"
)

type Event struct {
	Fields     map[string]string
	OccurredAt time.Time
	Level      string
	Message    string
}

type Sink interface {
	Emit(ctx context.Context, event Event) error
}

type MemorySink struct {
	mu     sync.Mutex
	events []Event
	err    error
}

func NewMemorySink() *MemorySink {
	return &MemorySink{}
}

func (sink *MemorySink) Emit(_ context.Context, event Event) error {
	if sink == nil {
		return nil
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.events = append(sink.events, event)
	return sink.err
}

func (sink *MemorySink) Events() []Event {
	if sink == nil {
		return nil
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	events := make([]Event, len(sink.events))
	copy(events, sink.events)
	return events
}

func (sink *MemorySink) SetError(err error) {
	if sink == nil {
		return
	}
	sink.mu.Lock()
	sink.err = err
	sink.mu.Unlock()
}
