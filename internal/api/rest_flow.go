package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"gestalt/internal/flow"
)

type flowConfigResponse struct {
	flow.Config
	StoragePath string `json:"storage_path,omitempty"`
}

type flowEventTypesResponse struct {
	EventTypes []string `json:"event_types"`
}

func (h *RestHandler) handleFlowActivities(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	writeJSON(w, http.StatusOK, flow.ActivityCatalog())
	return nil
}

func (h *RestHandler) handleFlowEventTypes(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	writeJSON(w, http.StatusOK, flowEventTypesResponse{
		EventTypes: flowEventTypes(),
	})
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
	writeJSON(w, http.StatusOK, buildFlowConfigResponse(cfg, h.FlowService))
	return nil
}

func (h *RestHandler) handleFlowConfigPut(w http.ResponseWriter, r *http.Request) *apiError {
	payload, apiErr := decodeFlowConfigPayload(r)
	if apiErr != nil {
		return apiErr
	}
	updated, err := h.FlowService.SaveConfig(r.Context(), payload)
	if err != nil {
		return mapFlowError(err)
	}
	writeJSON(w, http.StatusOK, buildFlowConfigResponse(updated, h.FlowService))
	return nil
}

func (h *RestHandler) handleFlowConfigExport(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireFlowService(); err != nil {
		return err
	}
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	cfg, err := h.FlowService.LoadConfig()
	if err != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to load flow config"}
	}
	payload, err := flow.EncodeFlowBundleYAML(cfg)
	if err != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to export flow config"}
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"flows.yaml\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
	return nil
}

func (h *RestHandler) handleFlowConfigImport(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireFlowService(); err != nil {
		return err
	}
	if r.Method != http.MethodPost {
		return methodNotAllowed(w, "POST")
	}
	payload, apiErr := decodeFlowImportPayload(r, h.FlowService.ActivityCatalog())
	if apiErr != nil {
		return apiErr
	}
	updated, err := h.FlowService.SaveConfig(r.Context(), payload)
	if err != nil {
		return mapFlowError(err)
	}
	writeJSON(w, http.StatusOK, buildFlowConfigResponse(updated, h.FlowService))
	return nil
}

func decodeFlowImportPayload(r *http.Request, defs []flow.ActivityDef) (flow.Config, *apiError) {
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if contentType == "" {
		return flow.Config{}, &apiError{Status: http.StatusUnsupportedMediaType, Message: "unsupported content type"}
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !isSupportedFlowYAMLMediaType(mediaType) {
		return flow.Config{}, &apiError{Status: http.StatusUnsupportedMediaType, Message: "unsupported content type"}
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return flow.Config{}, &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return flow.Config{}, &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}
	cfg, err := flow.DecodeFlowBundleYAML(body, defs)
	if err != nil {
		var validationErr *flow.ValidationError
		if errors.As(err, &validationErr) {
			if validationErr.Kind == flow.ValidationConflict {
				return flow.Config{}, &apiError{Status: http.StatusConflict, Message: validationErr.Message}
			}
		}
		return flow.Config{}, &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}
	return cfg, nil
}

func isSupportedFlowYAMLMediaType(mediaType string) bool {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "application/yaml", "application/x-yaml", "text/yaml", "text/x-yaml":
		return true
	default:
		return false
	}
}

func flowConfigStoragePath(service *flow.Service) string {
	if service == nil {
		return ""
	}
	return service.ConfigPath()
}

func buildFlowConfigResponse(cfg flow.Config, service *flow.Service) flowConfigResponse {
	return flowConfigResponse{
		Config:      cfg,
		StoragePath: flowConfigStoragePath(service),
	}
}

func decodeFlowConfigPayload(r *http.Request) (flow.Config, *apiError) {
	var payload flow.Config
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return flow.Config{}, &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}
	return payload, nil
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
	if errors.Is(err, flow.ErrDispatcherUnavailable) {
		return &apiError{Status: http.StatusServiceUnavailable, Message: "flow dispatcher unavailable"}
	}
	return &apiError{Status: http.StatusInternalServerError, Message: "failed to save flow config"}
}

func flowEventTypes() []string {
	coreTypes := []string{
		"file_changed",
		"git_branch_changed",
		"terminal_resized",
	}
	notifyTypes := flowNotifyTypeList()
	return append(coreTypes, notifyTypes...)
}

func flowNotifyTypeList() []string {
	values := []string{
		flow.CanonicalNotifyEventType("new-plan"),
		flow.CanonicalNotifyEventType("progress"),
		flow.CanonicalNotifyEventType("finish"),
		flow.CanonicalNotifyEventType("other"),
	}
	return uniqueStrings(values)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
