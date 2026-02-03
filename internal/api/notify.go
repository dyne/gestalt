package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func decodeNotifyRequest(r *http.Request) (notifyRequest, *apiError) {
	var request notifyRequest
	if r.Body == nil {
		return request, &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil && err != io.EOF {
		return request, &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}
	if err := validateNotifyRequest(&request); err != nil {
		return request, err
	}
	return request, nil
}

func validateNotifyRequest(request *notifyRequest) *apiError {
	if request == nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}
	if strings.TrimSpace(request.SessionID) == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "missing session id"}
	}
	if len(request.Payload) == 0 {
		return &apiError{Status: http.StatusUnprocessableEntity, Message: "missing payload"}
	}
	payloadType, err := extractNotifyPayloadType(request.Payload)
	if err != nil {
		return err
	}
	request.EventType = payloadType
	return nil
}

func extractNotifyPayloadType(payload json.RawMessage) (string, *apiError) {
	var payloadMap map[string]any
	if err := json.Unmarshal(payload, &payloadMap); err != nil || payloadMap == nil {
		return "", &apiError{Status: http.StatusUnprocessableEntity, Message: "payload must be a JSON object"}
	}
	rawType, ok := payloadMap["type"]
	if !ok {
		return "", &apiError{Status: http.StatusUnprocessableEntity, Message: "missing payload type"}
	}
	typeText, ok := rawType.(string)
	if !ok || strings.TrimSpace(typeText) == "" {
		return "", &apiError{Status: http.StatusUnprocessableEntity, Message: "missing payload type"}
	}
	return strings.TrimSpace(typeText), nil
}
