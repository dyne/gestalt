package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type AgentInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return e.Message
}

func FetchAgents(client *http.Client, baseURL, token string) ([]AgentInfo, error) {
	client = ensureClient(client)
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return nil, errors.New("base URL is required")
	}

	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/agents", nil)
	if err != nil {
		return nil, fmt.Errorf("build agents request failed: %w", err)
	}
	addToken(request, token)

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("agents request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		message := readErrorMessage(response)
		return nil, &HTTPError{StatusCode: response.StatusCode, Message: message}
	}

	var payload []AgentInfo
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode agents response: %w", err)
	}
	agents := make([]AgentInfo, 0, len(payload))
	for _, agent := range payload {
		id := strings.TrimSpace(agent.ID)
		name := strings.TrimSpace(agent.Name)
		if id == "" || name == "" {
			continue
		}
		agents = append(agents, AgentInfo{ID: id, Name: name})
	}
	return agents, nil
}

func SendAgentInput(client *http.Client, baseURL, token, agentName string, payload []byte) error {
	client = ensureClient(client)
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return errors.New("base URL is required")
	}
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		return errors.New("agent name is required")
	}

	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/agents/"+agentName+"/input", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request failed: %w", err)
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	addToken(request, token)

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		message := readErrorMessage(response)
		return &HTTPError{StatusCode: response.StatusCode, Message: message}
	}
	return nil
}

func StartAgent(client *http.Client, baseURL, token, agentID string) error {
	client = ensureClient(client)
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return errors.New("base URL is required")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return errors.New("agent id is required")
	}

	payload := map[string]string{"agent": agentID}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode start request: %w", err)
	}

	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/terminals", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build start request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	addToken(request, token)

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("start request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusCreated || response.StatusCode == http.StatusOK || response.StatusCode == http.StatusConflict {
		return nil
	}

	message := readErrorMessage(response)
	return &HTTPError{StatusCode: response.StatusCode, Message: message}
}

func ensureClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return http.DefaultClient
}

func addToken(request *http.Request, token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	request.Header.Set("Authorization", "Bearer "+token)
}

func readErrorMessage(response *http.Response) string {
	if response == nil {
		return "request failed"
	}
	body, _ := io.ReadAll(response.Body)
	text := strings.TrimSpace(string(body))
	if text == "" {
		return response.Status
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if strings.TrimSpace(payload.Error) != "" {
			return payload.Error
		}
	}
	return text
}
