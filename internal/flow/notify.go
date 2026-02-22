package flow

import "strings"

func CanonicalNotifyEventType(raw string) string {
	normalized := normalizeNotifyToken(raw)
	if normalized == "" {
		return "agent-turn-complete"
	}
	switch normalized {
	case "new-plan", "plan-new":
		return "plan-new"
	case "plan-update":
		return "plan-update"
	case "progress":
		return "plan-update"
	case "start", "work-start":
		return "work-start"
	case "work-progress":
		return "work-progress"
	case "finish", "work-finish":
		return "work-finish"
	case "git-commit", "commit":
		return "git-commit"
	case "prompt-voice":
		return "prompt-voice"
	case "prompt-text":
		return "prompt-text"
	default:
		if strings.HasPrefix(normalized, "plan-") {
			return "plan-update"
		}
		if strings.HasPrefix(normalized, "work-") {
			switch strings.TrimPrefix(normalized, "work-") {
			case "start":
				return "work-start"
			case "progress":
				return "work-progress"
			case "finish":
				return "work-finish"
			}
		}
		if strings.HasPrefix(normalized, "prompt-voice") {
			return "prompt-voice"
		}
		if strings.HasPrefix(normalized, "prompt-text") {
			return "prompt-text"
		}
		if strings.HasPrefix(normalized, "agent-turn") {
			return "agent-turn-complete"
		}
		return "agent-turn-complete"
	}
}

func normalizeNotifyToken(raw string) string {
	lower := strings.ToLower(raw)
	var builder strings.Builder
	builder.Grow(len(lower))
	lastDash := false
	for _, runeValue := range lower {
		if (runeValue >= 'a' && runeValue <= 'z') || (runeValue >= '0' && runeValue <= '9') {
			builder.WriteRune(runeValue)
			lastDash = false
			continue
		}
		if lastDash {
			continue
		}
		builder.WriteByte('-')
		lastDash = true
	}
	return strings.Trim(builder.String(), "-")
}
