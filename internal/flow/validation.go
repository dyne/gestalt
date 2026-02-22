package flow

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

type ValidationKind string

const (
	ValidationBadRequest ValidationKind = "bad_request"
	ValidationConflict   ValidationKind = "conflict"
)

type ValidationError struct {
	Kind    ValidationKind
	Message string
}

func (err *ValidationError) Error() string {
	if err == nil {
		return ""
	}
	return err.Message
}

func ValidateConfig(cfg Config, activityDefs []ActivityDef) error {
	if cfg.Version != 0 && cfg.Version != ConfigVersion {
		return &ValidationError{Kind: ValidationBadRequest, Message: fmt.Sprintf("unsupported config version %d", cfg.Version)}
	}

	index := activityIndex(activityDefs)
	triggerIDs := map[string]struct{}{}
	for _, trigger := range cfg.Triggers {
		id := strings.TrimSpace(trigger.ID)
		if id == "" {
			return &ValidationError{Kind: ValidationBadRequest, Message: "trigger id is required"}
		}
		if strings.TrimSpace(trigger.EventType) == "" {
			return &ValidationError{Kind: ValidationBadRequest, Message: fmt.Sprintf("trigger %q event_type is required", id)}
		}
		if err := validateTriggerWhere(trigger); err != nil {
			return err
		}
		if _, exists := triggerIDs[id]; exists {
			return &ValidationError{Kind: ValidationConflict, Message: fmt.Sprintf("duplicate trigger id %q", id)}
		}
		triggerIDs[id] = struct{}{}
	}

	for triggerID, bindings := range cfg.BindingsByTriggerID {
		seen := map[string]struct{}{}
		for _, binding := range bindings {
			activityID := strings.TrimSpace(binding.ActivityID)
			if activityID == "" {
				return &ValidationError{Kind: ValidationBadRequest, Message: fmt.Sprintf("trigger %q activity_id is required", triggerID)}
			}
			if _, exists := seen[activityID]; exists {
				return &ValidationError{Kind: ValidationConflict, Message: fmt.Sprintf("duplicate activity %q for trigger %q", activityID, triggerID)}
			}
			seen[activityID] = struct{}{}
			def, ok := index[activityID]
			if !ok {
				return &ValidationError{Kind: ValidationConflict, Message: fmt.Sprintf("unknown activity_id %q", activityID)}
			}
			if err := validateActivityConfig(def, binding.Config); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateTriggerWhere(trigger EventTrigger) error {
	if len(trigger.Where) == 0 {
		return nil
	}
	if !isNotifyEventType(trigger.EventType) {
		return nil
	}
	allowed := map[string]struct{}{
		"session.id": {},
	}
	for key := range trigger.Where {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "" {
			return &ValidationError{Kind: ValidationBadRequest, Message: fmt.Sprintf("trigger %q where key is required", trigger.ID)}
		}
		if _, ok := allowed[normalized]; !ok {
			return &ValidationError{Kind: ValidationBadRequest, Message: fmt.Sprintf("trigger %q where key %q is not supported for notify events", trigger.ID, key)}
		}
	}
	return nil
}

func isNotifyEventType(eventType string) bool {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "plan-new", "plan-update", "work-start", "work-progress", "work-finish", "agent-turn", "prompt-voice", "prompt-text":
		return true
	default:
		return false
	}
}

func validateActivityConfig(def ActivityDef, config map[string]any) error {
	if config == nil {
		config = map[string]any{}
	}
	for _, field := range def.Fields {
		value, ok := config[field.Key]
		if !ok || value == nil {
			if field.Required {
				return &ValidationError{Kind: ValidationBadRequest, Message: fmt.Sprintf("%s.%s is required", def.ID, field.Key)}
			}
			continue
		}
		if !fieldTypeMatches(field.Type, value) {
			return &ValidationError{Kind: ValidationBadRequest, Message: fmt.Sprintf("%s.%s must be %s", def.ID, field.Key, field.Type)}
		}
	}
	return nil
}

func fieldTypeMatches(fieldType string, value any) bool {
	switch fieldType {
	case "string":
		_, ok := value.(string)
		return ok
	case "bool":
		_, ok := value.(bool)
		return ok
	case "int":
		switch typed := value.(type) {
		case int, int32, int64:
			return true
		case float32:
			return float32(int64(typed)) == typed
		case float64:
			return math.Trunc(typed) == typed
		case json.Number:
			_, err := typed.Int64()
			return err == nil
		default:
			return false
		}
	default:
		return false
	}
}

func activityIndex(defs []ActivityDef) map[string]ActivityDef {
	if len(defs) == 0 {
		return map[string]ActivityDef{}
	}
	index := make(map[string]ActivityDef, len(defs))
	for _, def := range defs {
		if def.ID == "" {
			continue
		}
		index[def.ID] = def
	}
	return index
}
