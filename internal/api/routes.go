package api

import (
	"net/http"

	"gestalt/internal/terminal"
)

func RegisterRoutes(mux *http.ServeMux, manager *terminal.Manager, authToken string, staticDir string) {
	rest := &RestHandler{
		Manager: manager,
	}

	mux.Handle("/ws/terminal/", &TerminalHandler{
		Manager:   manager,
		AuthToken: authToken,
	})

	mux.Handle("/api/status", loggingMiddleware(restHandler(authToken, rest.handleStatus)))
	mux.Handle("/api/agents", loggingMiddleware(restHandler(authToken, rest.handleAgents)))
	mux.Handle("/api/terminals", loggingMiddleware(restHandler(authToken, rest.handleTerminals)))
	mux.Handle("/api/terminals/", loggingMiddleware(restHandler(authToken, rest.handleTerminal)))

	if staticDir != "" {
		mux.Handle("/", NewSPAHandler(staticDir))
		return
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if authToken != "" {
			w.Header().Set("X-Gestalt-Auth", "required")
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("gestalt ok\n"))
	})
}
