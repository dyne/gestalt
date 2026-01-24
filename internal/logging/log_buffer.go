package logging

import (
	"sync"

	"gestalt/internal/buffer"
)

type LogBuffer struct {
	mu      sync.Mutex
	entries *buffer.Ring[LogEntry]
}

func NewLogBuffer(size int) *LogBuffer {
	return &LogBuffer{
		entries: buffer.NewRing[LogEntry](size),
	}
}

func (b *LogBuffer) Add(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.entries == nil {
		return
	}

	b.entries.Add(entry)
}

func (b *LogBuffer) List() []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.entries.List()
}
