package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"gestalt/internal/flow"
)

type flowConfigResponse struct {
	flow.Config
	TemporalStatus *flowTemporalStatus `json:"temporal_status,omitempty"`
}

type flowTemporalStatus struct {
	Enabled bool `json:"enabled"`
}

func (h *RestHandler) handleFlowActivities(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	writeJSON(w, http.StatusOK, flow.ActivityCatalog())
	return nil
}

func (h *RestHandler) handleFlowConfig(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireFlowService(); err != nil {
		return err
	}
	switch r.Method {
	case http.MethodGet:
		return h.handleFlowConfigGet(w, r)
	case http.MethodPut:
		return h.handleFlowConfigPut(w, r)
	default:
		return methodNotAllowed(w, "GET, PUT")
	}
}

func (h *RestHandler) handleFlowConfigGet(w http.ResponseWriter, r *http.Request) *apiError {
	cfg, err := h.FlowService.LoadConfig()
	if err != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to load flow config"}
	}
	writeJSON(w, http.StatusOK, flowConfigResponse{
		Config:         cfg,
		TemporalStatus: flowStatus(h.FlowService),
	})
	return nil
}

func (h *RestHandler) handleFlowConfigPut(w http.ResponseWriter, r *http.Request) *apiError {
	var payload flow.Config
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}
	updated, err := h.FlowService.SaveConfig(r.Context(), payload)
	if err != nil {
		return mapFlowError(err)
	}
	writeJSON(w, http.StatusOK, flowConfigResponse{
		Config:         updated,
		TemporalStatus: flowStatus(h.FlowService),
	})
	return nil
}

func flowStatus(service *flow.Service) *flowTemporalStatus {
	if service == nil {
		return nil
	}
	return &flowTemporalStatus{Enabled: service.TemporalAvailable()}
}

func mapFlowError(err error) *apiError {
	if err == nil {
		return nil
	}
	var validation *flow.ValidationError
	if errors.As(err, &validation) {
		switch validation.Kind {
		case flow.ValidationConflict:
			return &apiError{Status: http.StatusConflict, Message: validation.Message}
		default:
			return &apiError{Status: http.StatusBadRequest, Message: validation.Message}
		}
	}
	if errors.Is(err, flow.ErrTemporalUnavailable) {
		return &apiError{Status: http.StatusServiceUnavailable, Message: "temporal unavailable"}
	}
	return &apiError{Status: http.StatusInternalServerError, Message: "failed to save flow config"}
}
