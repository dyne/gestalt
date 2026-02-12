package terminal

import (
	"bytes"
	"testing"
)

type suffixFilter struct {
	suffix      []byte
	flushData   []byte
	resizeCalls [][2]uint16
	stats       OutputFilterStats
}

func (f *suffixFilter) Write(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	out := make([]byte, 0, len(data)+len(f.suffix))
	out = append(out, data...)
	out = append(out, f.suffix...)
	return out
}

func (f *suffixFilter) Flush() []byte {
	return f.flushData
}

func (f *suffixFilter) Resize(cols, rows uint16) {
	f.resizeCalls = append(f.resizeCalls, [2]uint16{cols, rows})
}

func (f *suffixFilter) Reset() {}

func (f *suffixFilter) Stats() OutputFilterStats {
	return f.stats
}

func TestFilterChainWritePassesThrough(t *testing.T) {
	t.Parallel()

	chain := NewFilterChain(
		&suffixFilter{suffix: []byte("-a")},
		&suffixFilter{suffix: []byte("-b")},
	)

	got := chain.Write([]byte("start"))
	if !bytes.Equal(got, []byte("start-a-b")) {
		t.Fatalf("expected chained output, got %q", got)
	}
}

func TestFilterChainWriteNoFilters(t *testing.T) {
	t.Parallel()

	chain := NewFilterChain()
	input := []byte("plain")
	got := chain.Write(input)
	if !bytes.Equal(got, input) {
		t.Fatalf("expected passthrough output, got %q", got)
	}
}

func TestFilterChainFlushPassesThrough(t *testing.T) {
	t.Parallel()

	chain := NewFilterChain(
		&suffixFilter{flushData: []byte("flush")},
		&suffixFilter{suffix: []byte("-ok")},
	)

	got := chain.Flush()
	if !bytes.Equal(got, []byte("flush-ok")) {
		t.Fatalf("expected flushed output, got %q", got)
	}
}

func TestFilterChainResizePropagates(t *testing.T) {
	t.Parallel()

	first := &suffixFilter{}
	second := &suffixFilter{}
	chain := NewFilterChain(first, second)

	chain.Resize(80, 24)

	if len(first.resizeCalls) != 1 || len(second.resizeCalls) != 1 {
		t.Fatalf("expected resize on each filter, got %d and %d", len(first.resizeCalls), len(second.resizeCalls))
	}
	if first.resizeCalls[0] != [2]uint16{80, 24} || second.resizeCalls[0] != [2]uint16{80, 24} {
		t.Fatalf("unexpected resize values: %v %v", first.resizeCalls[0], second.resizeCalls[0])
	}
}

func TestFilterChainStatsAggregates(t *testing.T) {
	t.Parallel()

	chain := NewFilterChain(
		&suffixFilter{stats: OutputFilterStats{InBytes: 10, OutBytes: 8, DroppedBytes: 2}},
		&suffixFilter{stats: OutputFilterStats{InBytes: 8, OutBytes: 6, DroppedBytes: 2}},
	)

	stats := chain.Stats()
	if stats.FilterName != "filter-chain" {
		t.Fatalf("expected filter-chain name, got %q", stats.FilterName)
	}
	if stats.InBytes != 18 || stats.OutBytes != 14 || stats.DroppedBytes != 4 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}
