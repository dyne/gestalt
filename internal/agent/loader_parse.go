package agent

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

func loadAgentFromBytes(filePath string, data []byte) (Agent, error) {
	agent, err := parseAgentData(filePath, data)
	if err != nil {
		return Agent{}, formatParseError(filePath, err)
	}
	if err := agent.Validate(); err != nil {
		return Agent{}, formatValidationError(agent, filePath, data, err)
	}
	agent.ConfigHash = ComputeConfigHash(&agent)
	return agent, nil
}

func parseAgentData(filePath string, data []byte) (Agent, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	var agent Agent

	if ext != ".toml" {
		return Agent{}, fmt.Errorf("unsupported agent config extension %q", ext)
	}
	if _, err := toml.Decode(string(data), &agent); err != nil {
		return Agent{}, err
	}
	return agent, nil
}

func formatParseError(filePath string, err error) error {
	if err == nil {
		return nil
	}
	var parseErr toml.ParseError
	if errors.As(err, &parseErr) {
		return fmt.Errorf("parse agent file %s: %s", filePath, parseErr.ErrorWithPosition())
	}
	return fmt.Errorf("parse agent file %s: %w", filePath, err)
}

func formatValidationError(agent Agent, filePath string, data []byte, err error) error {
	var vErr *ValidationError
	if errors.As(err, &vErr) {
		path := strings.TrimSpace(vErr.Path)
		if path != "" {
			path = "cli_config." + path
		}
		line := lineForKey(data, path)
		location := filepath.Base(filePath)
		if agent.Name != "" {
			location = fmt.Sprintf("agent %q in %s", agent.Name, location)
		}
		if line > 0 {
			location = fmt.Sprintf("%s:%d", location, line)
		}

		message := vErr.Message
		if message == "" {
			message = fmt.Sprintf("expected %s, got %s", vErr.Expected, vErr.Actual)
		}
		if path != "" {
			message = fmt.Sprintf("%s: %s", path, message)
		}
		return fmt.Errorf("%s: %s", location, message)
	}

	location := filepath.Base(filePath)
	if agent.Name != "" {
		location = fmt.Sprintf("agent %q in %s", agent.Name, location)
	}
	return fmt.Errorf("%s: %v", location, err)
}

func lineForKey(data []byte, keyPath string) int {
	key := strings.TrimSpace(keyPath)
	if key == "" {
		return 0
	}
	if idx := strings.LastIndex(key, "."); idx != -1 {
		key = key[idx+1:]
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if strings.Contains(text, key) && (strings.Contains(text, "=") || strings.Contains(text, ":")) {
			return line
		}
	}
	return 0
}
