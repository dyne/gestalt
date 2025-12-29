package api

import (
	"net/http"

	"gestalt/internal/terminal"
)

func RegisterRoutes(mux *http.ServeMux, manager *terminal.Manager, authToken string, staticDir string) {
	rest := &RestHandler{
		Manager:   manager,
		AuthToken: authToken,
	}

	mux.Handle("/ws/terminal/", &TerminalHandler{
		Manager:   manager,
		AuthToken: authToken,
	})

	mux.HandleFunc("/api/status", rest.handleStatus)
	mux.HandleFunc("/api/terminals", rest.handleTerminals)
	mux.HandleFunc("/api/terminals/", rest.handleTerminal)

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
