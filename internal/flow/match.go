package flow

import "strings"

func MatchTrigger(trigger EventTrigger, normalized map[string]string) bool {
	if normalized == nil {
		return false
	}
	if trigger.EventType != "" {
		eventType, ok := normalized["type"]
		if !ok || !strings.EqualFold(eventType, trigger.EventType) {
			return false
		}
	}
	for key, expected := range trigger.Where {
		normalizedValue, ok := normalized[strings.ToLower(strings.TrimSpace(key))]
		if !ok {
			return false
		}
		if !strings.EqualFold(normalizedValue, expected) {
			return false
		}
	}
	return true
}
