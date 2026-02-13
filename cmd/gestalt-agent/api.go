package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gestalt/internal/runner/launchspec"
)

type createSessionRequest struct {
	Agent  string `json:"agent"`
	Runner string `json:"runner"`
}

type createSessionResponse struct {
	ID     string                 `json:"id"`
	Runner string                 `json:"runner"`
	Launch *launchspec.LaunchSpec `json:"launch"`
}

func createExternalSession(client *http.Client, baseURL, token, agentID string) (*createSessionResponse, error) {
	client = ensureHTTPClient(client)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("server URL is required")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, errors.New("agent id is required")
	}

	payload := createSessionRequest{Agent: agentID, Runner: "external"}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode create session request: %w", err)
	}

	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/sessions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build create session request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	addToken(request, token)

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("create session request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		message := readResponseMessage(response.Body)
		if message == "" {
			message = fmt.Sprintf("create session failed with status %d", response.StatusCode)
		}
		return nil, errors.New(message)
	}

	var session createSessionResponse
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("decode create session response: %w", err)
	}
	return &session, nil
}

func ensureHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		return http.DefaultClient
	}
	return client
}

func addToken(request *http.Request, token string) {
	token = strings.TrimSpace(token)
	if token == "" || request == nil {
		return
	}
	request.Header.Set("Authorization", "Bearer "+token)
}

func readResponseMessage(body io.Reader) string {
	if body == nil {
		return ""
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
