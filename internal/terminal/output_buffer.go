package terminal

import (
	"strings"
	"sync"
)

const DefaultBufferLines = 1000

type OutputBuffer struct {
	mu       sync.Mutex
	maxLines int
	lines    []string
	carry    string
}

func NewOutputBuffer(maxLines int) *OutputBuffer {
	if maxLines <= 0 {
		maxLines = DefaultBufferLines
	}

	return &OutputBuffer{maxLines: maxLines}
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

	lines := make([]string, len(b.lines))
	copy(lines, b.lines)
	if b.carry != "" {
		lines = append(lines, b.carry)
	}

	return lines
}

func (b *OutputBuffer) appendLine(line string) {
	b.lines = append(b.lines, line)
	if len(b.lines) > b.maxLines {
		drop := len(b.lines) - b.maxLines
		b.lines = b.lines[drop:]
	}
}
