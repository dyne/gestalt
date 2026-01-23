package temporal

import (
	"bytes"
	"fmt"
	"strings"

	"gestalt/internal/agent"

	"github.com/BurntSushi/toml"
)

const memoLimitBytes = 2048

func SerializeAgentConfig(profile *agent.Agent) (string, error) {
	if profile == nil {
		return "", nil
	}
	var buffer bytes.Buffer
	if err := toml.NewEncoder(&buffer).Encode(profile); err != nil {
		return "", err
	}
	return truncateMemo(buffer.Bytes()), nil
}

func DeserializeAgentConfig(data string) (*agent.Agent, error) {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return nil, fmt.Errorf("agent config is empty")
	}
	if looksLikeJSONMemo(trimmed) {
		return nil, fmt.Errorf("legacy JSON memo detected; TOML agent config is required")
	}
	var profile agent.Agent
	if _, err := toml.Decode(trimmed, &profile); err != nil {
		return nil, fmt.Errorf("unable to parse agent config: %w", err)
	}
	return &profile, nil
}

func truncateMemo(data []byte) string {
	if len(data) <= memoLimitBytes {
		return string(data)
	}
	if memoLimitBytes <= 3 {
		return string(data[:memoLimitBytes])
	}
	return string(data[:memoLimitBytes-3]) + "..."
}

func looksLikeJSONMemo(data string) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return false
	}
	switch trimmed[0] {
	case '{', '[':
		return true
	default:
		return false
	}
}
