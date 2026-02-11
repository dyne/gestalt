package flow

import "strings"

func RenderTemplate(template string, request ActivityRequest) string {
	if template == "" {
		return ""
	}
	var builder strings.Builder
	remaining := template
	for {
		start := strings.Index(remaining, "{{")
		if start < 0 {
			builder.WriteString(remaining)
			break
		}
		builder.WriteString(remaining[:start])
		remaining = remaining[start+2:]
		end := strings.Index(remaining, "}}")
		if end < 0 {
			builder.WriteString("{{")
			builder.WriteString(remaining)
			break
		}
		token := strings.TrimSpace(remaining[:end])
		builder.WriteString(resolveTemplateToken(token, request))
		remaining = remaining[end+2:]
	}
	return builder.String()
}

func resolveTemplateToken(token string, request ActivityRequest) string {
	if token == "" {
		return ""
	}
	switch token {
	case "event_id":
		return request.EventID
	case "trigger_id":
		return request.TriggerID
	case "activity_id":
		return request.ActivityID
	case "output_tail":
		return request.OutputTail
	}
	if strings.HasPrefix(token, "event.") {
		key := strings.TrimPrefix(token, "event.")
		if key == "" {
			return ""
		}
		return request.Event[key]
	}
	return request.Event[token]
}
