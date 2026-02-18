package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gestalt/internal/flow"
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
		EventTypes   []string            `json:"event_types"`
		NotifyTypes  map[string]string   `json:"notify_types"`
		NotifyTokens map[string][]string `json:"notify_tokens"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.EventTypes) == 0 {
		t.Fatalf("expected event types")
	}
	if payload.NotifyTypes["new-plan"] != "notify_new_plan" {
		t.Fatalf("unexpected notify type mapping: %#v", payload.NotifyTypes)
	}
	if len(payload.NotifyTokens["notify_new_plan"]) == 0 {
		t.Fatalf("expected notify tokens")
	}
}

func TestFlowConfigEndpointGet(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "automations.json")
	repo := flow.NewFileRepository(configPath, nil)
	cfg := flow.Config{
		Version: flow.ConfigVersion,
		Triggers: []flow.EventTrigger{
			{ID: "t1", Label: "Trigger one", EventType: "workflow_paused", Where: map[string]string{"terminal_id": "t1"}},
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
	if got.StoragePath != configPath {
		t.Fatalf("expected storage path %q, got %q", configPath, got.StoragePath)
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
			{ID: "t1", Label: "Trigger one", EventType: "workflow_paused", Where: map[string]string{"terminal_id": "t1"}},
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
			{ID: "dup", EventType: "workflow_paused"},
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
			{ID: "t1", EventType: "workflow_paused"},
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
			{ID: "t1", Label: "Trigger one", EventType: "workflow_paused", Where: map[string]string{"terminal_id": "t1"}},
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
	if disposition := rec.Header().Get("Content-Disposition"); disposition == "" {
		t.Fatalf("expected content disposition header")
	}
	var got flow.Config
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Triggers) != 1 || got.Triggers[0].ID != "t1" {
		t.Fatalf("unexpected config: %#v", got.Triggers)
	}
}

func TestFlowConfigImportEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	repo := flow.NewFileRepository(filepath.Join(tempDir, "automations.json"), nil)
	service := flow.NewService(repo, nil, nil)
	handler := &RestHandler{FlowService: service}

	cfg := flow.Config{
		Version: flow.ConfigVersion,
		Triggers: []flow.EventTrigger{
			{ID: "t1", Label: "Trigger one", EventType: "workflow_paused", Where: map[string]string{"terminal_id": "t1"}},
		},
		BindingsByTriggerID: map[string][]flow.ActivityBinding{},
	}
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/flow/config/import", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleFlowConfigImport)(rec, req)

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

func TestFlowConfigImportEndpointValidationErrors(t *testing.T) {
	tempDir := t.TempDir()
	repo := flow.NewFileRepository(filepath.Join(tempDir, "automations.json"), nil)
	service := flow.NewService(repo, nil, nil)
	handler := &RestHandler{FlowService: service}

	req := httptest.NewRequest(http.MethodPost, "/api/flow/config/import", bytes.NewBufferString("{"))
	rec := httptest.NewRecorder()
	restHandler("", nil, handler.handleFlowConfigImport)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	duplicate := flow.Config{
		Version: flow.ConfigVersion,
		Triggers: []flow.EventTrigger{
			{ID: "dup", EventType: "workflow_paused"},
			{ID: "dup", EventType: "file_changed"},
		},
		BindingsByTriggerID: map[string][]flow.ActivityBinding{},
	}
	body, _ := json.Marshal(duplicate)
	req = httptest.NewRequest(http.MethodPost, "/api/flow/config/import", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	restHandler("", nil, handler.handleFlowConfigImport)(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rec.Code)
	}
}
