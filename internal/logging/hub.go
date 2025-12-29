package logging

import "sync"

const defaultSubscriberBuffer = 100

type LogHub struct {
	mu     sync.Mutex
	nextID uint64
	subs   map[uint64]chan LogEntry
	closed bool
}

func NewLogHub() *LogHub {
	return &LogHub{
		subs: make(map[uint64]chan LogEntry),
	}
}

func (h *LogHub) Subscribe(buffer int) (<-chan LogEntry, func()) {
	if h == nil {
		return nil, func() {}
	}
	if buffer <= 0 {
		buffer = defaultSubscriberBuffer
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		ch := make(chan LogEntry)
		close(ch)
		return ch, func() {}
	}
	h.nextID++
	id := h.nextID
	ch := make(chan LogEntry, buffer)
	h.subs[id] = ch
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if existing, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(existing)
		}
	}
}

func (h *LogHub) Broadcast(entry LogEntry) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	subs := make([]chan LogEntry, 0, len(h.subs))
	for _, ch := range h.subs {
		subs = append(subs, ch)
	}
	h.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- entry:
		default:
		}
	}
}

func (h *LogHub) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for id, ch := range h.subs {
		delete(h.subs, id)
		close(ch)
	}
}
