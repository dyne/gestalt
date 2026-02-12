package terminal

import (
	"strings"

	"gestalt/internal/agent"
)

// BuildOutputFilterChain resolves and instantiates the configured output filters.
func BuildOutputFilterChain(profile *agent.Agent, runtimeInterface string) OutputFilter {
	names := ResolveOutputFilterNames(profile, runtimeInterface)
	if len(names) == 0 {
		return NewFilterChain()
	}
	filters := make([]OutputFilter, 0, len(names))
	for _, name := range names {
		if filter := outputFilterForName(name); filter != nil {
			filters = append(filters, filter)
		}
	}
	if len(filters) == 0 {
		return NewFilterChain()
	}
	return NewFilterChain(filters...)
}

func outputFilterForName(name string) OutputFilter {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "ansi-strip":
		return NewANSIStripFilter()
	case "utf8-guard":
		return NewUTF8GuardFilter()
	case "scrollback-vt":
		return NewScrollbackVTFilter()
	case "codex-tui":
		return NewCodexTUIFilter()
	default:
		return nil
	}
}
