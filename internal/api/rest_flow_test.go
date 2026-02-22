package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gestalt/internal/flow"
	"gopkg.in/yaml.v3"
)

func TestFlowActivitiesEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/flow/activities", nil)
	rec := httptest.NewRecorder()

	handler := &RestHandler{}
	restHandler("", nil, handler.handleFlowActivities)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var defs []flow.ActivityDef
	if err := json.Unmarshal(rec.Body.Bytes(), &defs); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(defs) == 0 {
		t.Fatalf("expected activity catalog")
	}
}

func TestFlowEventTypesEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/flow/event-types", nil)
	rec := httptest.NewRecorder()

	handler := &RestHandler{}
	restHandler("", nil, handler.handleFlowEventTypes)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload struct {
		EventTypes []string `json:"event_types"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.EventTypes) == 0 {
		t.Fatalf("expected event types")
	}
}

func TestFlowConfigEndpointGet(t *testing.T) {
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "automations.json")
	repo := flow.NewFileRepository(repoPath, nil)
	cfg := flow.Config{
		Version: flow.ConfigVersion,
		Triggers: []flow.EventTrigger{
			{ID: "t1", Label: "Trigger one", EventType: "file_changed", Where: map[string]string{"terminal_id": "t1"}},
		},
		BindingsByTriggerID: map[string][]flow.ActivityBinding{
			"t1": {
				{ActivityID: "toast_notification", Config: map[string]any{"level": "info", "message_template": "hi"}},
			},
		},
	}
	if err := repo.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	service := flow.NewService(repo, nil, nil)
	handler := &RestHandler{FlowService: service}

	req := httptest.NewRequest(http.MethodGet, "/api/flow/config", nil)
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleFlowConfig)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var got flowConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.StoragePath != tempDir {
		t.Fatalf("expected storage path %q, got %q", tempDir, got.StoragePath)
	}
	if len(got.Triggers) != 1 || got.Triggers[0].ID != "t1" {
		t.Fatalf("unexpected config: %#v", got.Triggers)
	}
}

func TestFlowConfigEndpointPut(t *testing.T) {
	tempDir := t.TempDir()
	repo := flow.NewFileRepository(filepath.Join(tempDir, "automations.json"), nil)
	service := flow.NewService(repo, nil, nil)
	handler := &RestHandler{FlowService: service}

	cfg := flow.Config{
		Version: flow.ConfigVersion,
		Triggers: []flow.EventTrigger{
			{ID: "t1", Label: "Trigger one", EventType: "file_changed", Where: map[string]string{"terminal_id": "t1"}},
		},
		BindingsByTriggerID: map[string][]flow.ActivityBinding{
			"t1": {
				{ActivityID: "toast_notification", Config: map[string]any{"level": "info", "message_template": "hi"}},
			},
		},
	}
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/flow/config", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleFlowConfig)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	loaded, err := repo.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(loaded.Triggers) != 1 || loaded.Triggers[0].ID != "t1" {
		t.Fatalf("unexpected persisted config: %#v", loaded.Triggers)
	}
}

func TestFlowConfigEndpointValidationErrors(t *testing.T) {
	tempDir := t.TempDir()
	repo := flow.NewFileRepository(filepath.Join(tempDir, "automations.json"), nil)
	service := flow.NewService(repo, nil, nil)
	handler := &RestHandler{FlowService: service}

	duplicate := flow.Config{
		Version: flow.ConfigVersion,
		Triggers: []flow.EventTrigger{
			{ID: "dup", EventType: "file_changed"},
			{ID: "dup", EventType: "file_changed"},
		},
		BindingsByTriggerID: map[string][]flow.ActivityBinding{},
	}
	body, _ := json.Marshal(duplicate)
	req := httptest.NewRequest(http.MethodPut, "/api/flow/config", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleFlowConfig)(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rec.Code)
	}

	invalidType := flow.Config{
		Version: flow.ConfigVersion,
		Triggers: []flow.EventTrigger{
			{ID: "t1", EventType: "file_changed"},
		},
		BindingsByTriggerID: map[string][]flow.ActivityBinding{
			"t1": {
				{ActivityID: "send_to_terminal", Config: map[string]any{"target_agent_name": "alpha", "message_template": "hi", "output_tail_lines": "nope"}},
			},
		},
	}
	body, _ = json.Marshal(invalidType)
	req = httptest.NewRequest(http.MethodPut, "/api/flow/config", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	restHandler("", nil, handler.handleFlowConfig)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestFlowConfigExportEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	repo := flow.NewFileRepository(filepath.Join(tempDir, "automations.json"), nil)
	cfg := flow.Config{
		Version: flow.ConfigVersion,
		Triggers: []flow.EventTrigger{
			{ID: "t1", Label: "Trigger one", EventType: "file_changed", Where: map[string]string{"terminal_id": "t1"}},
		},
		BindingsByTriggerID: map[string][]flow.ActivityBinding{},
	}
	if err := repo.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	service := flow.NewService(repo, nil, nil)
	handler := &RestHandler{FlowService: service}

	req := httptest.NewRequest(http.MethodGet, "/api/flow/config/export", nil)
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleFlowConfigExport)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/yaml; charset=utf-8" {
		t.Fatalf("expected yaml content type, got %q", contentType)
	}
	if disposition := rec.Header().Get("Content-Disposition"); disposition != "attachment; filename=\"flows.yaml\"" {
		t.Fatalf("expected content disposition for flows.yaml, got %q", disposition)
	}
	var got flow.FlowBundle
	if err := yaml.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Flows) != 1 || got.Flows[0].ID != "t1" {
		t.Fatalf("unexpected config: %#v", got.Flows)
	}
}

func TestFlowConfigImportEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	repo := flow.NewFileRepository(filepath.Join(tempDir, "automations.json"), nil)
	service := flow.NewService(repo, nil, nil)
	handler := &RestHandler{FlowService: service}

	payload := []byte(`
version: 1
flows:
  - id: t1
    label: Trigger one
    event_type: file_changed
    where:
      terminal_id: t1
    bindings: []
`)

	acceptedTypes := []string{
		"application/yaml",
		"application/x-yaml",
		"text/yaml",
		"text/x-yaml",
		"text/yaml; charset=utf-8",
	}
	for _, mediaType := range acceptedTypes {
		req := httptest.NewRequest(http.MethodPost, "/api/flow/config/import", bytes.NewReader(payload))
		req.Header.Set("Content-Type", mediaType)
		rec := httptest.NewRecorder()
		restHandler("", nil, handler.handleFlowConfigImport)(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("media type %q: expected status 200, got %d: %s", mediaType, rec.Code, rec.Body.String())
		}
		loaded, err := repo.Load()
		if err != nil {
			t.Fatalf("media type %q: load config: %v", mediaType, err)
		}
		if len(loaded.Triggers) != 1 || loaded.Triggers[0].ID != "t1" {
			t.Fatalf("media type %q: unexpected persisted config: %#v", mediaType, loaded.Triggers)
		}
	}
}

func TestFlowConfigImportEndpointValidationErrors(t *testing.T) {
	tempDir := t.TempDir()
	repo := flow.NewFileRepository(filepath.Join(tempDir, "automations.json"), nil)
	service := flow.NewService(repo, nil, nil)
	handler := &RestHandler{FlowService: service}

	req := httptest.NewRequest(http.MethodPost, "/api/flow/config/import", bytes.NewBufferString("{"))
	req.Header.Set("Content-Type", "text/yaml")
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleFlowConfigImport)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	duplicate := []byte(`
version: 1
flows:
  - id: dup
    event_type: file_changed
    bindings: []
  - id: dup
    event_type: file_changed
    bindings: []
`)
	req = httptest.NewRequest(http.MethodPost, "/api/flow/config/import", bytes.NewReader(duplicate))
	req.Header.Set("Content-Type", "application/yaml")
	rec = httptest.NewRecorder()
	restHandler("", nil, handler.handleFlowConfigImport)(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/flow/config/import", bytes.NewReader(duplicate))
	rec = httptest.NewRecorder()
	restHandler("", nil, handler.handleFlowConfigImport)(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected status 415 for missing content type, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/flow/config/import", bytes.NewReader(duplicate))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	restHandler("", nil, handler.handleFlowConfigImport)(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected status 415 for unsupported content type, got %d", rec.Code)
	}
}
