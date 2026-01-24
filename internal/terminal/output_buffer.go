package terminal

import (
	"strings"
	"sync"

	"gestalt/internal/buffer"
)

const DefaultBufferLines = 1000

type OutputBuffer struct {
	mu       sync.Mutex
	maxLines int
	lines    *buffer.Ring[string]
	carry    string
}

func NewOutputBuffer(maxLines int) *OutputBuffer {
	if maxLines <= 0 {
		maxLines = DefaultBufferLines
	}

	return &OutputBuffer{
		maxLines: maxLines,
		lines:    buffer.NewRing[string](maxLines),
	}
}

func (b *OutputBuffer) Append(data []byte) {
	if len(data) == 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	chunk := b.carry + string(data)
	parts := strings.Split(chunk, "\n")
	if len(parts) == 0 {
		return
	}

	if chunk[len(chunk)-1] != '\n' {
		b.carry = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	} else {
		b.carry = ""
	}

	for _, line := range parts {
		b.appendLine(line)
	}
}

func (b *OutputBuffer) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	lines := b.lines.List()
	if lines == nil {
		lines = []string{}
	}
	if b.carry != "" {
		lines = append(lines, b.carry)
	}

	return lines
}

func (b *OutputBuffer) appendLine(line string) {
	if b.lines == nil {
		b.lines = buffer.NewRing[string](b.maxLines)
	}
	b.lines.Add(line)
}
