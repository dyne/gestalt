package logging

import "sync"

type LogBuffer struct {
	mu      sync.Mutex
	entries []LogEntry
	start   int
	count   int
}

func NewLogBuffer(size int) *LogBuffer {
	if size <= 0 {
		size = 1
	}
	return &LogBuffer{
		entries: make([]LogEntry, size),
	}
}

func (b *LogBuffer) Add(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.entries) == 0 {
		return
	}

	if b.count < len(b.entries) {
		index := (b.start + b.count) % len(b.entries)
		b.entries[index] = entry
		b.count++
		return
	}

	b.entries[b.start] = entry
	b.start = (b.start + 1) % len(b.entries)
}

func (b *LogBuffer) List() []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.count == 0 {
		return nil
	}

	out := make([]LogEntry, b.count)
	for i := 0; i < b.count; i++ {
		index := (b.start + i) % len(b.entries)
		out[i] = b.entries[index]
	}
	return out
}
