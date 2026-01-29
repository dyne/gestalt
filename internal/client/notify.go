package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type NotifyRequest struct {
	TerminalID string          `json:"terminal_id"`
	AgentID    string          `json:"agent_id"`
	AgentName  string          `json:"agent_name,omitempty"`
	Source     string          `json:"source"`
	EventType  string          `json:"event_type"`
	OccurredAt *time.Time      `json:"occurred_at,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Raw        string          `json:"raw,omitempty"`
	EventID    string          `json:"event_id,omitempty"`
}

func PostNotifyEvent(client *http.Client, baseURL, token, terminalID string, payload NotifyRequest) error {
	client = ensureClient(client)
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return errors.New("base URL is required")
	}
	terminalID = strings.TrimSpace(terminalID)
	if terminalID == "" {
		return errors.New("terminal id is required")
	}

	if payload.TerminalID != "" && payload.TerminalID != terminalID {
		return fmt.Errorf("terminal id mismatch")
	}
	payload.TerminalID = terminalID

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode notify request: %w", err)
	}

	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/terminals/"+terminalID+"/notify", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build notify request failed: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	addToken(request, token)

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("notify request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusOK {
		return nil
	}

	message := readErrorMessage(response)
	return &HTTPError{StatusCode: response.StatusCode, Message: message}
}
