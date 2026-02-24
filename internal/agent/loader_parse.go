package agent

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"gestalt/internal/config/tomlkeys"
	internalschema "gestalt/internal/schema"

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
	if err := agent.NormalizeShell(); err != nil {
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
	raw, err := tomlkeys.DecodeMap(data)
	if err != nil {
		return Agent{}, err
	}
	if _, err := toml.Decode(string(data), &agent); err != nil {
		return Agent{}, err
	}
	applyCLIConfig(&agent, raw)
	applyModelAlias(&agent, raw, filePath)
	if err := ValidateAgentConfig(agent.CLIType, agent.CLIConfig); err != nil {
		return Agent{}, err
	}
	if agent.Singleton == nil {
		defaultSingleton := true
		agent.Singleton = &defaultSingleton
	}
	if agent.Singleton != nil && !*agent.Singleton {
		message := "agent singleton=false is deprecated and ignored; singleton sessions are always enforced"
		agent.warnings = append(agent.warnings, message)
		emitConfigValidationErrorWithMessage(filePath, message)
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
		line := lineForKey(data, path)
		location := filepath.Base(filePath)
		if agent.Name != "" {
			location = fmt.Sprintf("agent %q in %s", agent.Name, location)
		}
		if line > 0 {
			location = fmt.Sprintf("%s:%d", location, line)
		}

		actualDetail := internalschema.FormatActualDetail(vErr.Actual, vErr.ActualValue)
		message := vErr.Message
		if message == "" {
			message = fmt.Sprintf("expected %s, got %s", vErr.Expected, actualDetail)
		}
		if message == "unknown field" {
			actualValue := internalschema.FormatValidationValue(vErr.ActualValue)
			if actualValue != "" {
				message = fmt.Sprintf("%s (value=%s)", message, actualValue)
			}
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

var reservedAgentKeys = []string{
	"name",
	"shell",
	"prompt",
	"skills",
	"onair_string",
	"singleton",
	"interface",
	"cli_type",
	"cli_config",
	"codex_mode",
	"model",
	"llm_model",
	"hidden",
}

func applyCLIConfig(agent *Agent, raw map[string]interface{}) {
	if agent == nil {
		return
	}
	agent.CLIType = strings.ToLower(strings.TrimSpace(rawString(raw["cli_type"])))
	agent.Interface = strings.ToLower(strings.TrimSpace(rawString(raw["interface"])))
	if strings.TrimSpace(agent.Interface) == "" {
		agent.Interface = AgentInterfaceCLI
	}

	cliConfig := map[string]interface{}{}
	if rawConfig, ok := raw["cli_config"].(map[string]interface{}); ok {
		for key, value := range rawConfig {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			cliConfig[trimmed] = value
		}
	}
	for key, value := range raw {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" || slices.Contains(reservedAgentKeys, trimmed) {
			continue
		}
		cliConfig[trimmed] = value
	}
	if agent.Model != "" && agent.CLIType != "" {
		if _, ok := cliConfig["model"]; !ok {
			cliConfig["model"] = agent.Model
		}
	}
	if len(cliConfig) > 0 {
		agent.CLIConfig = cliConfig
	}
}

func applyModelAlias(agent *Agent, raw map[string]interface{}, filePath string) {
	modelValue := strings.TrimSpace(rawString(raw["model"]))
	llmModelValue := strings.TrimSpace(rawString(raw["llm_model"]))

	if modelValue != "" {
		agent.Model = modelValue
	} else if llmModelValue != "" {
		agent.Model = llmModelValue
	}

	if llmModelValue != "" {
		message := "agent llm_model is deprecated; use model"
		agent.warnings = append(agent.warnings, message)
		emitConfigValidationErrorWithMessage(filePath, message)
	}

}

func rawString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}
