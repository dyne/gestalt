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
	if err := validateNotifyRequest(request); err != nil {
		return request, err
	}
	return request, nil
}

func validateNotifyRequest(request notifyRequest) *apiError {
	if strings.TrimSpace(request.SessionID) == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "missing session id"}
	}
	if strings.TrimSpace(request.AgentID) == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "missing agent id"}
	}
	if strings.TrimSpace(request.Source) == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "missing source"}
	}
	if strings.TrimSpace(request.EventType) == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "missing event type"}
	}
	return nil
}
