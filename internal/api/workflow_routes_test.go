package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gestalt/internal/terminal"
)

func TestWorkflowRoutesReturnNotFound(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	mux := http.NewServeMux()
	RegisterRoutes(mux, manager, "", StatusConfig{}, "", nil, nil, nil, nil)

	tests := []struct {
		name string
		path string
	}{
		{name: "workflows", path: "/api/workflows"},
		{name: "workflow-events", path: "/api/workflows/events"},
		{name: "workflow-resume", path: "/api/sessions/123/workflow/resume"},
		{name: "workflow-history", path: "/api/sessions/123/workflow/history"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, test.path, nil)
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, req)
			if res.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d", res.Code)
			}
		})
	}
}
