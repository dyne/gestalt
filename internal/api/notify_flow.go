package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"gestalt/internal/flow"
	"gestalt/internal/terminal"
)

func buildNotifyFlowFields(session *terminal.Session, request notifyRequest, now time.Time) (map[string]string, *apiError) {
	if session == nil {
		return nil, &apiError{Status: http.StatusBadRequest, Message: "session not found"}
	}
	var payload map[string]any
	if err := json.Unmarshal(request.Payload, &payload); err != nil || payload == nil {
		return nil, &apiError{Status: http.StatusUnprocessableEntity, Message: "payload must be a JSON object"}
	}

	timestamp := now.UTC()
	if request.OccurredAt != nil && !request.OccurredAt.IsZero() {
		timestamp = request.OccurredAt.UTC()
	}

	fields := flow.BuildNotifyFields(flow.NotifyFieldInput{
		SessionID:   session.ID,
		AgentID:     session.AgentID,
		AgentName:   session.AgentName(),
		EventID:     request.EventID,
		PayloadType: request.EventType,
		OccurredAt:  timestamp,
		Payload:     payload,
	})
	return fields, nil
}

func buildNotifyLogFields(base map[string]string, request notifyRequest) map[string]string {
	fields := map[string]string{}
	for key, value := range base {
		fields[key] = value
	}
	fields["gestalt.category"] = "notification"
	fields["gestalt.source"] = "notify"
	if sessionID := fields["session_id"]; sessionID != "" {
		fields["session.id"] = sessionID
	}
	if agentID := fields["agent_id"]; agentID != "" {
		fields["agent.id"] = agentID
	}
	if agentName := fields["agent_name"]; agentName != "" {
		fields["agent.name"] = agentName
	}
	if eventID := strings.TrimSpace(request.EventID); eventID != "" {
		fields["notify.event_id"] = eventID
	}
	if fields["notify.type"] == "" {
		fields["notify.type"] = request.EventType
	}
	return fields
}
