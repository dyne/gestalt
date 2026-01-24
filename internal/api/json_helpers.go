package api

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, err *apiError) {
	if err == nil {
		return
	}
	writeJSON(w, err.Status, errorResponse{
		Error:      err.Message,
		TerminalID: err.TerminalID,
	})
}
