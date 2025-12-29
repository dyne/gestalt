package terminal

import (
	"sync"
	"sync/atomic"
)

// Broadcaster fans out output to multiple subscribers without blocking on slow listeners.
type Broadcaster struct {
	mu          sync.Mutex
	subscribers map[uint64]chan []byte
	nextSubID   uint64
	buffer      *OutputBuffer
	closed      bool
	closeOnce   sync.Once
}

func NewBroadcaster(bufferLines int) *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[uint64]chan []byte),
		buffer:      NewOutputBuffer(bufferLines),
	}
}

func (b *Broadcaster) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 128)
	id := atomic.AddUint64(&b.nextSubID, 1)

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	b.subscribers[id] = ch
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if existing, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(existing)
		}
		b.mu.Unlock()
	}

	return ch, cancel
}

func (b *Broadcaster) Broadcast(chunk []byte) {
	if len(chunk) == 0 {
		return
	}

	b.buffer.Append(chunk)

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	for _, ch := range b.subscribers {
		select {
		case ch <- chunk:
		default:
		}
	}
}

func (b *Broadcaster) OutputLines() []string {
	return b.buffer.Lines()
}

func (b *Broadcaster) Close() {
	b.closeOnce.Do(func() {
		b.mu.Lock()
		b.closed = true
		for id, ch := range b.subscribers {
			delete(b.subscribers, id)
			close(ch)
		}
		b.mu.Unlock()
	})
}
