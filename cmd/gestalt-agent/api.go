package main

import (
	"net/http"

	clientapi "gestalt/internal/client"
)

type createSessionResponse struct {
	ID string `json:"id"`
}

func createExternalSession(client *http.Client, baseURL, token, agentID string) (*createSessionResponse, error) {
	session, err := clientapi.CreateExternalAgentSession(client, baseURL, token, agentID)
	if err != nil {
		return nil, err
	}
	return &createSessionResponse{ID: session.ID}, nil
}
