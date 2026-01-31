package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
)

// ComputeConfigHash returns a stable FNV-1a hash for an agent's configuration.
func ComputeConfigHash(agent *Agent) string {
	if agent == nil {
		return ""
	}
	payload := map[string]interface{}{
		"name":         agent.Name,
		"shell":        agent.Shell,
		"codex_mode":   agent.CodexMode,
		"prompt":       []string(agent.Prompts),
		"skills":       agent.Skills,
		"onair_string": agent.OnAirString,
		"use_workflow": agent.UseWorkflow,
		"singleton":    agent.Singleton,
		"cli_type":     agent.CLIType,
		"llm_model":    agent.LLMModel,
		"cli_config":   agent.CLIConfig,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	var normalized interface{}
	if err := json.Unmarshal(jsonPayload, &normalized); err != nil {
		return ""
	}

	canonical, err := marshalCanonical(normalized)
	if err != nil {
		return ""
	}
	checksum := fnv.New64a()
	_, _ = checksum.Write(canonical)
	return fmt.Sprintf("%016x", checksum.Sum64())
}

func marshalCanonical(value interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	if err := writeCanonical(&buffer, value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func writeCanonical(buffer *bytes.Buffer, value interface{}) error {
	switch typed := value.(type) {
	case nil:
		buffer.WriteString("null")
		return nil
	case bool:
		if typed {
			buffer.WriteString("true")
		} else {
			buffer.WriteString("false")
		}
		return nil
	case string:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return err
		}
		buffer.Write(encoded)
		return nil
	case float64:
		buffer.WriteString(strconv.FormatFloat(typed, 'f', -1, 64))
		return nil
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		buffer.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				buffer.WriteByte(',')
			}
			encodedKey, err := json.Marshal(key)
			if err != nil {
				return err
			}
			buffer.Write(encodedKey)
			buffer.WriteByte(':')
			if err := writeCanonical(buffer, typed[key]); err != nil {
				return err
			}
		}
		buffer.WriteByte('}')
		return nil
	case []interface{}:
		buffer.WriteByte('[')
		for i, entry := range typed {
			if i > 0 {
				buffer.WriteByte(',')
			}
			if err := writeCanonical(buffer, entry); err != nil {
				return err
			}
		}
		buffer.WriteByte(']')
		return nil
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return err
		}
		buffer.Write(encoded)
		return nil
	}
}
