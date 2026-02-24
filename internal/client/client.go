package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type AgentInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SessionID string `json:"session_id"`
	Running   bool   `json:"running"`
}

type SessionInfo struct {
	ID string `json:"id"`
}

type HTTPError struct {
	StatusCode int
	Message    string
}

var defaultWaitSessionReadyTimeout = 2 * time.Second

// ChatSessionRef is the special session reference used for chat-only messages.
const ChatSessionRef = "chat"

func (e *HTTPError) Error() string {
	return e.Message
}

// NormalizeSessionRef canonicalizes session references used by CLI tools.
// Explicit session references are preserved; only surrounding whitespace is trimmed.
func NormalizeSessionRef(ref string) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", errors.New("session reference is required")
	}
	return trimmed, nil
}

// IsChatSessionRef reports whether the reference targets the special chat session.
func IsChatSessionRef(ref string) bool {
	return strings.EqualFold(strings.TrimSpace(ref), ChatSessionRef)
}

func ResolveSessionRef(ref string) (string, error) {
	return NormalizeSessionRef(ref)
}

func ResolveSessionRefAgainstSessions(ref string, sessions []SessionInfo) (string, error) {
	normalized, err := NormalizeSessionRef(ref)
	if err != nil {
		return "", err
	}
	if IsExplicitNumberedSessionRef(normalized) {
		return normalized, nil
	}
	if sessionExists(sessions, normalized) {
		return normalized, nil
	}
	canonical := normalized + " 1"
	if sessionExists(sessions, canonical) {
		return canonical, nil
	}
	return canonical, nil
}

func IsExplicitNumberedSessionRef(ref string) bool {
	fields := strings.Fields(strings.TrimSpace(ref))
	if len(fields) < 2 {
		return false
	}
	last := fields[len(fields)-1]
	if last == "" {
		return false
	}
	for _, r := range last {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func sessionExists(sessions []SessionInfo, id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, session := range sessions {
		if strings.TrimSpace(session.ID) == id {
			return true
		}
	}
	return false
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
		agents = append(agents, AgentInfo{
			ID:        id,
			Name:      name,
			SessionID: strings.TrimSpace(agent.SessionID),
			Running:   agent.Running,
		})
	}
	return agents, nil
}

func SendSessionInput(client *http.Client, baseURL, token, sessionID string, payload []byte) error {
	client = ensureClient(client)
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return errors.New("base URL is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is required")
	}

	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/sessions/"+sessionID+"/input", bytes.NewReader(payload))
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

func FetchSessions(client *http.Client, baseURL, token string) ([]SessionInfo, error) {
	client = ensureClient(client)
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return nil, errors.New("base URL is required")
	}

	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/sessions", nil)
	if err != nil {
		return nil, fmt.Errorf("build sessions request failed: %w", err)
	}
	addToken(request, token)

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("sessions request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		message := readErrorMessage(response)
		return nil, &HTTPError{StatusCode: response.StatusCode, Message: message}
	}

	var payload []SessionInfo
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode sessions response: %w", err)
	}
	sessions := make([]SessionInfo, 0, len(payload))
	for _, session := range payload {
		id := strings.TrimSpace(session.ID)
		if id == "" {
			continue
		}
		sessions = append(sessions, SessionInfo{ID: id})
	}
	return sessions, nil
}

// CreateExternalAgentSession starts a session for agentID using the external runner.
func CreateExternalAgentSession(client *http.Client, baseURL, token, agentID string) (SessionInfo, error) {
	client = ensureClient(client)
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return SessionInfo{}, errors.New("base URL is required")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return SessionInfo{}, errors.New("agent id is required")
	}

	payload := map[string]string{"agent": agentID, "runner": "external"}
	body, err := json.Marshal(payload)
	if err != nil {
		return SessionInfo{}, fmt.Errorf("encode create request: %w", err)
	}

	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/sessions", bytes.NewReader(body))
	if err != nil {
		return SessionInfo{}, fmt.Errorf("build create request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	addToken(request, token)

	response, err := client.Do(request)
	if err != nil {
		return SessionInfo{}, fmt.Errorf("create request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		message := readErrorMessage(response)
		return SessionInfo{}, &HTTPError{StatusCode: response.StatusCode, Message: message}
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		return SessionInfo{}, fmt.Errorf("decode create response: %w", err)
	}
	sessionID := strings.TrimSpace(created.ID)
	if sessionID == "" {
		return SessionInfo{}, errors.New("session id is missing from create response")
	}
	return SessionInfo{ID: sessionID}, nil
}

func WaitSessionReady(client *http.Client, baseURL, token, sessionID string, timeout time.Duration) error {
	client = ensureClient(client)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if timeout <= 0 {
		timeout = defaultWaitSessionReadyTimeout
	}
	deadline := time.Now().Add(timeout)
	for {
		sessions, err := FetchSessions(client, baseURL, token)
		if err != nil {
			return err
		}
		for _, session := range sessions {
			if session.ID == sessionID {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for session %q", sessionID)
		}
		time.Sleep(50 * time.Millisecond)
	}
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
