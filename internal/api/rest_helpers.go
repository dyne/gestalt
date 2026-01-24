package api

import "net/http"

func (h *RestHandler) requireManager() *apiError {
	if h.Manager == nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "terminal manager unavailable"}
	}
	return nil
}

func (h *RestHandler) requireLogger() *apiError {
	if h.Logger == nil || h.Logger.Buffer() == nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "log buffer unavailable"}
	}
	return nil
}
