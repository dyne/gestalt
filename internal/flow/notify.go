package flow

import "strings"

func CanonicalNotifyEventType(raw string) string {
	normalized := normalizeNotifyToken(raw)
	switch normalized {
	case "new_plan":
		return "notify_new_plan"
	case "progress":
		return "notify_progress"
	case "finish":
		return "notify_finish"
	default:
		return "notify_event"
	}
}

func normalizeNotifyToken(raw string) string {
	lower := strings.ToLower(raw)
	var builder strings.Builder
	builder.Grow(len(lower))
	lastUnderscore := false
	for _, runeValue := range lower {
		if (runeValue >= 'a' && runeValue <= 'z') || (runeValue >= '0' && runeValue <= '9') {
			builder.WriteRune(runeValue)
			lastUnderscore = false
			continue
		}
		if lastUnderscore {
			continue
		}
		builder.WriteByte('_')
		lastUnderscore = true
	}
	return strings.Trim(builder.String(), "_")
}
