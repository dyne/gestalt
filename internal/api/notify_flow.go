package api

import (
	"encoding/json"
	"net/http"
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
		AgentName:   session.ID,
		EventID:     request.EventID,
		PayloadType: request.EventType,
		OccurredAt:  timestamp,
		Payload:     payload,
	})
	return fields, nil
}
