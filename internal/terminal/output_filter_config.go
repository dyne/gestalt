package terminal

import (
	"os"
	"strconv"
	"strings"

	"gestalt/internal/agent"
)

const (
	envTerminalOutputFilters        = "GESTALT_TERMINAL_OUTPUT_FILTERS"
	envTerminalOutputFiltersDisable = "GESTALT_TERMINAL_OUTPUT_FILTERS_DISABLE"
)

// ResolveOutputFilterNames returns the filter chain names for a session.
func ResolveOutputFilterNames(profile *agent.Agent, runtimeInterface string) []string {
	if outputFiltersDisabled() {
		return nil
	}
	if envFilters := parseFilterList(os.Getenv(envTerminalOutputFilters)); len(envFilters) > 0 {
		return envFilters
	}
	if profile != nil {
		if filters := normalizeFilterList(profile.OutputFilters); len(filters) > 0 {
			return filters
		}
		if filter := strings.TrimSpace(profile.OutputFilter); filter != "" {
			return []string{filter}
		}
	}
	if !strings.EqualFold(runtimeInterface, agent.AgentInterfaceCLI) {
		return nil
	}
	if profile != nil && strings.EqualFold(strings.TrimSpace(profile.CLIType), "codex") {
		return []string{"scrollback-vt", "ansi-strip", "utf8-guard"}
	}
	return []string{"ansi-strip", "utf8-guard"}
}

func outputFiltersDisabled() bool {
	raw := strings.TrimSpace(os.Getenv(envTerminalOutputFiltersDisable))
	if raw == "" {
		return false
	}
	parsed, err := strconv.ParseBool(raw)
	return err == nil && parsed
}

func parseFilterList(raw string) []string {
	return normalizeFilterList(strings.Split(raw, ","))
}

func normalizeFilterList(filters []string) []string {
	cleaned := make([]string, 0, len(filters))
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if filter == "" {
			continue
		}
		cleaned = append(cleaned, filter)
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}
