package terminal

import (
	"strings"
	"testing"
)

// Benchmark results (2026-01-25, local dev):
// - FilterTerminalOutput/100B_Clean: 1865 ns/op
// - FilterTerminalOutput/1KB_Clean: 17137 ns/op
// - FilterTerminalOutput/10KB_Clean: 173055 ns/op
// - FilterTerminalOutput/1KB_ANSI: 18036 ns/op
// - StripRepeatedChars/1KB_Repeated: 889.9 ns/op

func benchmarkFilterTerminalOutput(b *testing.B, input string) {
	b.Helper()
	for i := 0; i < b.N; i++ {
		_ = FilterTerminalOutput(input)
	}
}

func BenchmarkFilterTerminalOutput_100B_Clean(b *testing.B) {
	benchmarkFilterTerminalOutput(b, buildString("clean text ", 100))
}

func BenchmarkFilterTerminalOutput_1KB_Clean(b *testing.B) {
	benchmarkFilterTerminalOutput(b, buildString("clean text ", 1024))
}

func BenchmarkFilterTerminalOutput_10KB_Clean(b *testing.B) {
	benchmarkFilterTerminalOutput(b, buildString("clean text ", 10*1024))
}

func BenchmarkFilterTerminalOutput_1KB_ANSI(b *testing.B) {
	benchmarkFilterTerminalOutput(b, buildString("value\x1b[31mred\x1b[0m ", 1024))
}

func BenchmarkStripRepeatedChars_1KB_Repeated(b *testing.B) {
	input := buildString("-----", 1024)
	for i := 0; i < b.N; i++ {
		_ = StripRepeatedChars(input, 3)
	}
}

func buildString(chunk string, target int) string {
	if target <= 0 {
		return ""
	}
	var builder strings.Builder
	builder.Grow(target)
	for builder.Len() < target {
		builder.WriteString(chunk)
	}
	result := builder.String()
	if len(result) <= target {
		return result
	}
	return result[:target]
}
