package terminal

import "strings"

// NewCodexTUIFilter drops known Codex TUI chrome lines from transcripts.
func NewCodexTUIFilter() OutputFilter {
	return &codexTUIFilter{
		stats: OutputFilterStats{FilterName: "codex-tui"},
	}
}

type codexTUIFilter struct {
	buffer []byte
	stats  OutputFilterStats
}

func (f *codexTUIFilter) Write(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	f.stats.InBytes += uint64(len(data))
	f.buffer = append(f.buffer, data...)

	var out []byte
	for {
		idx := indexByte(f.buffer, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(string(f.buffer[:idx]), "\r")
		f.buffer = f.buffer[idx+1:]
		if shouldDropCodexLine(line) {
			f.stats.DroppedBytes += uint64(len(line) + 1)
			continue
		}
		out = append(out, []byte(line)...)
		out = append(out, '\n')
	}
	f.stats.OutBytes += uint64(len(out))
	if len(out) == 0 {
		return nil
	}
	return out
}

func (f *codexTUIFilter) Flush() []byte {
	if len(f.buffer) == 0 {
		return nil
	}
	line := strings.TrimRight(string(f.buffer), "\r")
	f.buffer = nil
	if shouldDropCodexLine(line) {
		f.stats.DroppedBytes += uint64(len(line))
		return nil
	}
	f.stats.OutBytes += uint64(len(line))
	return []byte(line)
}

func (f *codexTUIFilter) Resize(cols, rows uint16) {}

func (f *codexTUIFilter) Reset() {
	f.buffer = nil
	f.stats = OutputFilterStats{FilterName: "codex-tui"}
}

func (f *codexTUIFilter) Stats() OutputFilterStats {
	return f.stats
}

func shouldDropCodexLine(line string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(line))
	if trimmed == "" {
		return false
	}
	patterns := []string{
		"openai codex",
		"context left",
		"tokens left",
		"press ctrl",
		"press c",
	}
	for _, pattern := range patterns {
		if strings.Contains(trimmed, pattern) {
			return true
		}
	}
	return false
}

func indexByte(data []byte, target byte) int {
	for i, b := range data {
		if b == target {
			return i
		}
	}
	return -1
}
