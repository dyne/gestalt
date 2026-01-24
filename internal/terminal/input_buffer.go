package terminal

import (
	"strings"
	"sync"
	"time"

	"gestalt/internal/buffer"
)

const DefaultInputBufferSize = 1000

type InputEntry struct {
	Command   string
	Timestamp time.Time
}

type InputBuffer struct {
	mu          sync.Mutex
	maxCommands int
	entries     *buffer.Ring[InputEntry]
}

func NewInputBuffer(maxCommands int) *InputBuffer {
	if maxCommands <= 0 {
		maxCommands = DefaultInputBufferSize
	}

	return &InputBuffer{
		maxCommands: maxCommands,
		entries:     buffer.NewRing[InputEntry](maxCommands),
	}
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

	if b.entries == nil {
		return []InputEntry{}
	}
	entries := b.entries.List()
	if entries == nil {
		return []InputEntry{}
	}
	return entries
}

func (b *InputBuffer) GetRecent(n int) []InputEntry {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.entries == nil {
		return []InputEntry{}
	}
	entries := b.entries.List()
	if entries == nil {
		return []InputEntry{}
	}
	if n <= 0 || n >= len(entries) {
		return entries
	}
	return entries[len(entries)-n:]
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

	if b.entries == nil {
		b.entries = buffer.NewRing[InputEntry](b.maxCommands)
	}
	b.entries.Add(entry)
}
