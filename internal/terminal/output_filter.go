package terminal

// OutputFilter transforms terminal output into a transcript-friendly stream.
type OutputFilter interface {
	Write([]byte) []byte
	Flush() []byte
	Resize(cols, rows uint16)
	Reset()
	Stats() OutputFilterStats
}

// OutputFilterStats records byte counts and a stable filter name.
type OutputFilterStats struct {
	InBytes      uint64
	OutBytes     uint64
	DroppedBytes uint64
	FilterName   string
}

// FilterChain pipes output through multiple filters in order.
type FilterChain struct {
	filters []OutputFilter
}

// NewFilterChain builds a chain from the provided filters.
func NewFilterChain(filters ...OutputFilter) *FilterChain {
	return &FilterChain{filters: filters}
}

// Write passes data through each filter in order.
func (c *FilterChain) Write(data []byte) []byte {
	if len(c.filters) == 0 {
		return data
	}
	out := data
	for _, filter := range c.filters {
		out = filter.Write(out)
		if len(out) == 0 {
			return nil
		}
	}
	return out
}

// Flush drains buffered data from each filter and pipes it through the chain.
func (c *FilterChain) Flush() []byte {
	if len(c.filters) == 0 {
		return nil
	}
	var out []byte
	for i, filter := range c.filters {
		flushed := filter.Flush()
		if len(flushed) == 0 {
			continue
		}
		segment := flushed
		for j := i + 1; j < len(c.filters); j++ {
			segment = c.filters[j].Write(segment)
			if len(segment) == 0 {
				break
			}
		}
		if len(segment) > 0 {
			out = append(out, segment...)
		}
	}
	return out
}

// Resize informs filters about terminal size changes.
func (c *FilterChain) Resize(cols, rows uint16) {
	for _, filter := range c.filters {
		filter.Resize(cols, rows)
	}
}

// Reset clears filter state.
func (c *FilterChain) Reset() {
	for _, filter := range c.filters {
		filter.Reset()
	}
}

// Stats aggregates stats for the chain.
func (c *FilterChain) Stats() OutputFilterStats {
	stats := OutputFilterStats{FilterName: "filter-chain"}
	for _, filter := range c.filters {
		filterStats := filter.Stats()
		stats.InBytes += filterStats.InBytes
		stats.OutBytes += filterStats.OutBytes
		stats.DroppedBytes += filterStats.DroppedBytes
	}
	return stats
}
