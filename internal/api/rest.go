package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"gestalt/internal/terminal"
)

type RestHandler struct {
	Manager   *terminal.Manager
	AuthToken string
}

type terminalSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"`
}

type terminalOutputResponse struct {
	ID    string   `json:"id"`
	Lines []string `json:"lines"`
}

type statusResponse struct {
	TerminalCount int       `json:"terminal_count"`
	ServerTime    time.Time `json:"server_time"`
}

type createTerminalRequest struct {
	Title string `json:"title"`
	Role  string `json:"role"`
}

func (h *RestHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r) {
		return
	}
	if h.Manager == nil {
		http.Error(w, "terminal manager unavailable", http.StatusInternalServerError)
		return
	}

	terminals := h.Manager.List()
	response := statusResponse{
		TerminalCount: len(terminals),
		ServerTime:    time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *RestHandler) handleTerminals(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r) {
		return
	}
	if h.Manager == nil {
		http.Error(w, "terminal manager unavailable", http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		infos := h.Manager.List()
		response := make([]terminalSummary, 0, len(infos))
		for _, info := range infos {
			response = append(response, terminalSummary{
				ID:        info.ID,
				Title:     info.Title,
				Role:      info.Role,
				CreatedAt: info.CreatedAt,
				Status:    info.Status,
			})
		}
		writeJSON(w, http.StatusOK, response)
	case http.MethodPost:
		var request createTerminalRequest
		if r.Body != nil {
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil && err.Error() != "EOF" {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
		}

		session, err := h.Manager.Create(request.Role, request.Title)
		if err != nil {
			http.Error(w, "failed to create terminal", http.StatusInternalServerError)
			return
		}

		info := session.Info()
		response := terminalSummary{
			ID:        info.ID,
			Title:     info.Title,
			Role:      info.Role,
			CreatedAt: info.CreatedAt,
			Status:    info.Status,
		}
		writeJSON(w, http.StatusCreated, response)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *RestHandler) handleTerminal(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r) {
		return
	}
	if h.Manager == nil {
		http.Error(w, "terminal manager unavailable", http.StatusInternalServerError)
		return
	}

	id, wantsOutput := parseTerminalPath(r.URL.Path)
	if id == "" {
		http.Error(w, "missing terminal id", http.StatusBadRequest)
		return
	}

	if wantsOutput {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		session, ok := h.Manager.Get(id)
		if !ok {
			http.Error(w, "terminal not found", http.StatusNotFound)
			return
		}

		response := terminalOutputResponse{
			ID:    id,
			Lines: session.OutputLines(),
		}
		writeJSON(w, http.StatusOK, response)
		return
	}

	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", "DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.Manager.Delete(id); err != nil {
		if err == terminal.ErrSessionNotFound {
			http.Error(w, "terminal not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to delete terminal", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *RestHandler) authorize(w http.ResponseWriter, r *http.Request) bool {
	if validateToken(r, h.AuthToken) {
		return true
	}

	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return false
}

func parseTerminalPath(path string) (string, bool) {
	trimmed := strings.TrimPrefix(path, "/api/terminals/")
	if trimmed == path {
		return "", false
	}

	if strings.HasSuffix(trimmed, "/output") {
		id := strings.TrimSuffix(trimmed, "/output")
		id = strings.TrimSuffix(id, "/")
		return id, true
	}

	trimmed = strings.TrimSuffix(trimmed, "/")
	return trimmed, false
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
