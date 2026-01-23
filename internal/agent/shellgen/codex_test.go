package shellgen

import (
	"reflect"
	"testing"
)

func TestBuildCodexCommandSimple(t *testing.T) {
	config := map[string]interface{}{
		"model":           "o3",
		"approval_policy": "never",
		"tui": map[string]interface{}{
			"scroll_mode": "wheel",
		},
	}
	got := BuildCodexCommand(config)
	want := []string{"codex", "-c", "approval_policy=never", "-c", "model=o3", "-c", "tui.scroll_mode=wheel"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildCodexCommandArrays(t *testing.T) {
	config := map[string]interface{}{
		"notify": []string{"email", "slack"},
	}
	got := BuildCodexCommand(config)
	want := []string{"codex", "-c", "notify=[\"email\",\"slack\"]"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildCodexCommandBooleans(t *testing.T) {
	config := map[string]interface{}{
		"show_raw_agent_reasoning": true,
	}
	got := BuildCodexCommand(config)
	want := []string{"codex", "-c", "show_raw_agent_reasoning=true"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildCodexCommandEscapes(t *testing.T) {
	config := map[string]interface{}{
		"instructions": "fix this now",
	}
	got := BuildCodexCommand(config)
	want := []string{"codex", "-c", "'instructions=fix this now'"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildCodexCommandSkipsEmptyValues(t *testing.T) {
	config := map[string]interface{}{
		"approval_policy": "never",
		"model":           "",
		"notify":          []string{""},
		"tui": map[string]interface{}{
			"scroll_mode": "",
		},
	}
	got := BuildCodexCommand(config)
	want := []string{"codex", "-c", "approval_policy=never"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
