package agent

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
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
	applyModelAlias(&agent, raw, filePath)
	cliConfig, err := extractCLIConfig(raw)
	if err != nil {
		return Agent{}, err
	}
	agent.CLIConfig = cliConfig
	applyModelCLIConfig(&agent)
	if agent.Singleton == nil {
		defaultSingleton := true
		agent.Singleton = &defaultSingleton
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

var agentRootKeys = map[string]struct{}{
	"name":         {},
	"shell":        {},
	"codex_mode":   {},
	"prompt":       {},
	"skills":       {},
	"gui_modules":  {},
	"onair_string": {},
	"singleton":    {},
	"interface":    {},
	"cli_type":     {},
	"model":        {},
	"llm_model":    {},
	"hidden":       {},
	"cli_config":   {},
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

func applyModelCLIConfig(agent *Agent) {
	modelValue := strings.TrimSpace(agent.Model)
	if modelValue == "" {
		return
	}
	cliType := strings.TrimSpace(agent.CLIType)
	if cliType == "" && len(agent.CLIConfig) == 0 {
		return
	}
	if agent.CLIConfig == nil {
		agent.CLIConfig = map[string]interface{}{}
	}
	if _, ok := agent.CLIConfig["model"]; ok {
		return
	}
	agent.CLIConfig["model"] = modelValue
}

func rawString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func extractCLIConfig(raw map[string]interface{}) (map[string]interface{}, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	config := map[string]interface{}{}
	if rawCLI, ok := raw["cli_config"]; ok {
		cliMap, ok := internalschema.AsStringMap(rawCLI)
		if !ok {
			return nil, fmt.Errorf("cli_config must be a table")
		}
		for key, value := range cliMap {
			config[key] = value
		}
	}
	for key, value := range raw {
		if _, reserved := agentRootKeys[key]; reserved {
			continue
		}
		config[key] = value
	}
	if len(config) == 0 {
		return nil, nil
	}
	return config, nil
}
