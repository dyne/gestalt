package terminal

import "unicode/utf8"

// NewUTF8GuardFilter buffers incomplete UTF-8 runes across chunk boundaries.
func NewUTF8GuardFilter() OutputFilter {
	return &utf8GuardFilter{
		stats: OutputFilterStats{FilterName: "utf8-guard"},
	}
}

type utf8GuardFilter struct {
	pending []byte
	stats   OutputFilterStats
}

func (f *utf8GuardFilter) Write(data []byte) []byte {
	if len(data) == 0 && len(f.pending) == 0 {
		return nil
	}
	f.stats.InBytes += uint64(len(data))

	buf := append(f.pending, data...)
	f.pending = nil

	out := make([]byte, 0, len(buf))
	for i := 0; i < len(buf); {
		if !utf8.FullRune(buf[i:]) {
			f.pending = append(f.pending, buf[i:]...)
			break
		}
		r, size := utf8.DecodeRune(buf[i:])
		if r == utf8.RuneError && size == 1 {
			out = utf8.AppendRune(out, utf8.RuneError)
			f.stats.DroppedBytes++
			i++
			continue
		}
		out = append(out, buf[i:i+size]...)
		i += size
	}
	f.stats.OutBytes += uint64(len(out))
	return out
}

func (f *utf8GuardFilter) Flush() []byte {
	if len(f.pending) == 0 {
		return nil
	}
	pending := f.pending
	f.pending = nil
	out := utf8.AppendRune(nil, utf8.RuneError)
	f.stats.OutBytes += uint64(len(out))
	f.stats.DroppedBytes += uint64(len(pending))
	return out
}

func (f *utf8GuardFilter) Resize(cols, rows uint16) {}

func (f *utf8GuardFilter) Reset() {
	f.pending = nil
	f.stats = OutputFilterStats{FilterName: "utf8-guard"}
}

func (f *utf8GuardFilter) Stats() OutputFilterStats {
	return f.stats
}
