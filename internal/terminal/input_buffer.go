package terminal

import (
	"strings"
	"sync"
	"time"
)

const DefaultInputBufferSize = 1000

type InputEntry struct {
	Command   string
	Timestamp time.Time
}

type InputBuffer struct {
	mu          sync.Mutex
	maxCommands int
	entries     []InputEntry
}

func NewInputBuffer(maxCommands int) *InputBuffer {
	if maxCommands <= 0 {
		maxCommands = DefaultInputBufferSize
	}

	return &InputBuffer{maxCommands: maxCommands}
}

func (b *InputBuffer) Append(command string) {
	b.AppendEntry(InputEntry{
		Command:   command,
		Timestamp: time.Now().UTC(),
	})
}

func (b *InputBuffer) GetAll() []InputEntry {
	b.mu.Lock()
	defer b.mu.Unlock()

	entries := make([]InputEntry, len(b.entries))
	copy(entries, b.entries)
	return entries
}

func (b *InputBuffer) GetRecent(n int) []InputEntry {
	b.mu.Lock()
	defer b.mu.Unlock()

	if n <= 0 || n >= len(b.entries) {
		entries := make([]InputEntry, len(b.entries))
		copy(entries, b.entries)
		return entries
	}

	start := len(b.entries) - n
	entries := make([]InputEntry, n)
	copy(entries, b.entries[start:])
	return entries
}

func (b *InputBuffer) AppendEntry(entry InputEntry) {
	entry.Command = strings.TrimSpace(entry.Command)
	if entry.Command == "" {
		return
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries = append(b.entries, entry)
	if len(b.entries) > b.maxCommands {
		drop := len(b.entries) - b.maxCommands
		b.entries = b.entries[drop:]
	}
}
