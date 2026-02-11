package api

import (
	"encoding/json"
	"net/http"
	"path"
	"strconv"
	"strings"
)

type planProgressPayload struct {
	PlanFile  string
	L1        string
	L2        string
	TaskState string
	TaskLevel int
}

func normalizePlanProgressPayload(payload json.RawMessage) (planProgressPayload, json.RawMessage, *apiError) {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil || raw == nil {
		return planProgressPayload{}, nil, &apiError{Status: http.StatusUnprocessableEntity, Message: "payload must be a JSON object"}
	}

	planFile, ok := raw["plan_file"].(string)
	if !ok || strings.TrimSpace(planFile) == "" {
		return planProgressPayload{}, nil, &apiError{Status: http.StatusUnprocessableEntity, Message: "missing plan_file"}
	}

	progress := planProgressPayload{
		PlanFile: normalizePlanFile(planFile),
	}
	raw["plan_file"] = progress.PlanFile

	if l1, ok := raw["l1"].(string); ok {
		progress.L1 = normalizeHeadingLabel(l1)
		if progress.L1 != "" {
			raw["l1"] = progress.L1
		}
	}
	if l2, ok := raw["l2"].(string); ok {
		progress.L2 = normalizeHeadingLabel(l2)
		if progress.L2 != "" {
			raw["l2"] = progress.L2
		}
	}
	if taskState, ok := raw["task_state"].(string); ok {
		progress.TaskState = strings.TrimSpace(taskState)
		if progress.TaskState != "" {
			raw["task_state"] = progress.TaskState
		}
	}
	if taskLevel, ok := parseTaskLevel(raw["task_level"]); ok {
		progress.TaskLevel = taskLevel
		raw["task_level"] = taskLevel
	}

	normalized, err := json.Marshal(raw)
	if err != nil {
		return planProgressPayload{}, nil, &apiError{Status: http.StatusInternalServerError, Message: "failed to normalize progress payload"}
	}

	return progress, normalized, nil
}

func normalizePlanFile(planFile string) string {
	trimmed := strings.TrimSpace(planFile)
	trimmed = strings.ReplaceAll(trimmed, "\\", "/")
	return path.Base(trimmed)
}

func normalizeHeadingLabel(value string) string {
	trimmed := strings.TrimSpace(strings.TrimLeft(value, "*"))
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return ""
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return ""
	}

	start := 0
	if isTodoKeyword(fields[0]) {
		start++
	}
	if start < len(fields) && isPriorityToken(fields[start]) {
		start++
	}
	if start >= len(fields) {
		return ""
	}
	return strings.TrimSpace(strings.Join(fields[start:], " "))
}

func isTodoKeyword(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "TODO", "WIP", "DONE":
		return true
	default:
		return false
	}
}

func isPriorityToken(value string) bool {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "[#") && strings.HasSuffix(trimmed, "]") && len(trimmed) >= 4 {
		return true
	}
	return false
}

func parseTaskLevel(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return parsed, true
	case int:
		return typed, true
	case int64:
		return int(typed), true
	default:
		return 0, false
	}
}
