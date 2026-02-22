package flow

import (
	"strconv"
	"strings"
)

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
		if strings.TrimSpace(expected) == "*" {
			continue
		}
		normalizedValue, ok := normalized[strings.ToLower(strings.TrimSpace(key))]
		if !ok {
			return false
		}
		if !matchWhereValue(key, normalizedValue, expected) {
			return false
		}
	}
	return true
}

// matchWhereValue handles trigger matching for specific keys like session ids.
func matchWhereValue(key, normalizedValue, expected string) bool {
	if strings.EqualFold(normalizedValue, expected) {
		return true
	}
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	if normalizedKey != "session.id" && normalizedKey != "session_id" {
		return false
	}
	if strings.TrimSpace(expected) == "" {
		return true
	}
	if hasSessionSequence(expected) {
		return false
	}
	return matchSessionPrefix(normalizedValue, expected)
}

// hasSessionSequence returns true when the session value ends with a numeric sequence.
func hasSessionSequence(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	idx := strings.LastIndex(trimmed, " ")
	if idx == -1 {
		return false
	}
	suffix := strings.TrimSpace(trimmed[idx+1:])
	if suffix == "" {
		return false
	}
	_, err := strconv.ParseUint(suffix, 10, 64)
	return err == nil
}

// matchSessionPrefix allows "name" to match "name 1" style session ids.
func matchSessionPrefix(actual, expected string) bool {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimSpace(expected)
	if actual == "" || expected == "" {
		return false
	}
	if strings.EqualFold(actual, expected) {
		return true
	}
	lowerActual := strings.ToLower(actual)
	lowerExpected := strings.ToLower(expected)
	prefix := lowerExpected + " "
	if !strings.HasPrefix(lowerActual, prefix) {
		return false
	}
	suffix := strings.TrimSpace(actual[len(expected):])
	if suffix == "" {
		return false
	}
	_, err := strconv.ParseUint(suffix, 10, 64)
	return err == nil
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
