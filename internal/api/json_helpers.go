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
	code := err.Code
	if code == "" {
		code = errorCodeForStatus(err.Status)
	}
	writeJSON(w, err.Status, errorResponse{
		Message:    err.Message,
		Error:      err.Message,
		Code:       code,
		TerminalID: err.TerminalID,
	})
}
