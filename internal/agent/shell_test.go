package agent

import "testing"

func TestBuildShellCommandCodex(t *testing.T) {
	config := map[string]interface{}{
		"model":           "o3",
		"approval_policy": "never",
		"tui": map[string]interface{}{
			"scroll_mode": "wheel",
		},
	}
	got := BuildShellCommand("codex", config)
	want := "codex -c approval_policy=never -c model=o3 -c tui.scroll_mode=wheel"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildShellCommandCopilot(t *testing.T) {
	config := map[string]interface{}{
		"allow_all_tools": true,
		"model":           "gpt-5",
	}
	got := BuildShellCommand("copilot", config)
	want := "copilot --allow-all-tools --model gpt-5"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildShellCommandCopilotFalseFlag(t *testing.T) {
	config := map[string]interface{}{
		"allow_all_tools": false,
	}
	got := BuildShellCommand("copilot", config)
	want := "copilot --no-allow-all-tools"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildShellCommandEscapesValues(t *testing.T) {
	config := map[string]interface{}{
		"prompt": "fix this now",
	}
	got := BuildShellCommand("copilot", config)
	want := "copilot --prompt 'fix this now'"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildShellCommandSkipsEmptyValues(t *testing.T) {
	config := map[string]interface{}{
		"approval_policy": "never",
		"model":           "",
	}
	got := BuildShellCommand("codex", config)
	want := "codex -c approval_policy=never"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
