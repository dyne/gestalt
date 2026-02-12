package launchspec

import (
	"strings"
	"time"
)

// RunnerKind describes the runner backend for a session.
type RunnerKind string

const (
	// RunnerKindServer uses a backend-managed PTY runner.
	RunnerKindServer RunnerKind = "server"
	// RunnerKindExternal uses an externally managed runner (for example tmux).
	RunnerKindExternal RunnerKind = "external"
)

// LaunchSpec describes how to launch and attach a runner.
type LaunchSpec struct {
	SessionID       string              `json:"session_id"`
	Argv            []string            `json:"argv"`
	Interface       string              `json:"interface"`
	PromptFiles     []string            `json:"prompt_files"`
	GUIModules      []string            `json:"gui_modules"`
	PromptInjection PromptInjectionSpec `json:"prompt_injection"`
}

// PromptInjectionMode describes how prompts should be injected.
type PromptInjectionMode string

const (
	// PromptInjectionNone disables injection (default).
	PromptInjectionNone PromptInjectionMode = "none"
	// PromptInjectionCodexDeveloperInstructions means prompt is embedded in CLI config.
	PromptInjectionCodexDeveloperInstructions PromptInjectionMode = "codex-developer-instructions"
	// PromptInjectionStdin streams prompt payloads over stdin with pacing.
	PromptInjectionStdin PromptInjectionMode = "stdin"
)

// PromptInjectionSpec configures prompt injection behavior.
type PromptInjectionSpec struct {
	Mode    PromptInjectionMode   `json:"mode"`
	Payload []string              `json:"payload"`
	Pacing  PromptInjectionPacing `json:"pacing"`
}

// PromptInjectionPacing mirrors the backend prompt injection delays.
type PromptInjectionPacing struct {
	PromptDelay      time.Duration `json:"prompt_delay"`
	InterPromptDelay time.Duration `json:"inter_prompt_delay"`
	FinalEnterDelay  time.Duration `json:"final_enter_delay"`
	EnterKeyDelay    time.Duration `json:"enter_key_delay"`
	OnAirTimeout     time.Duration `json:"on_air_timeout"`
}

// DefaultPromptInjectionPacing returns the default prompt injection pacing.
func DefaultPromptInjectionPacing() PromptInjectionPacing {
	return PromptInjectionPacing{
		PromptDelay:      3 * time.Second,
		InterPromptDelay: 100 * time.Millisecond,
		FinalEnterDelay:  500 * time.Millisecond,
		EnterKeyDelay:    75 * time.Millisecond,
		OnAirTimeout:     5 * time.Second,
	}
}

// NormalizePromptInjection normalizes prompt injection settings and defaults.
func NormalizePromptInjection(spec PromptInjectionSpec) PromptInjectionSpec {
	if strings.TrimSpace(string(spec.Mode)) == "" {
		spec.Mode = PromptInjectionNone
	}
	if spec.Mode == PromptInjectionStdin && spec.Pacing == (PromptInjectionPacing{}) {
		spec.Pacing = DefaultPromptInjectionPacing()
	}
	return spec
}

// NormalizeLaunchSpec normalizes a launch specification for consistent output.
func NormalizeLaunchSpec(spec LaunchSpec) LaunchSpec {
	spec.SessionID = strings.TrimSpace(spec.SessionID)
	spec.Interface = strings.TrimSpace(spec.Interface)
	spec.PromptFiles = normalizeList(spec.PromptFiles)
	spec.GUIModules = normalizeList(spec.GUIModules)
	spec.PromptInjection = NormalizePromptInjection(spec.PromptInjection)
	return spec
}

func normalizeList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
