package guimodules

import "strings"

const (
	ModuleConsole      = "console"
	ModulePlanProgress = "plan-progress"
)

// Normalize normalizes GUI module ids, de-duplicates, and preserves order.
// Legacy "terminal" is normalized to "console".
func Normalize(modules []string) []string {
	if len(modules) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(modules))
	normalized := make([]string, 0, len(modules))
	for _, entry := range modules {
		module := normalizeOne(entry)
		if module == "" {
			continue
		}
		if _, exists := seen[module]; exists {
			continue
		}
		seen[module] = struct{}{}
		normalized = append(normalized, module)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizeOne(value string) string {
	module := strings.ToLower(strings.TrimSpace(value))
	if module == "" {
		return ""
	}
	if module == "terminal" {
		return ModuleConsole
	}
	return module
}
