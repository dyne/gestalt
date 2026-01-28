package flow

import "strings"

type ActivityMatch struct {
	Trigger EventTrigger
	Binding ActivityBinding
}

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

func MatchBindings(config Config, normalized map[string]string) []ActivityMatch {
	if len(config.Triggers) == 0 {
		return []ActivityMatch{}
	}
	matches := []ActivityMatch{}
	for _, trigger := range config.Triggers {
		if !MatchTrigger(trigger, normalized) {
			continue
		}
		for _, binding := range config.BindingsByTriggerID[trigger.ID] {
			matches = append(matches, ActivityMatch{
				Trigger: trigger,
				Binding: binding,
			})
		}
	}
	return matches
}
