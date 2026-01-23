package shellgen

import (
	"reflect"
	"testing"
)

func TestBuildCopilotCommandSimple(t *testing.T) {
	config := map[string]interface{}{
		"allow_all_tools": true,
		"model":           "gpt-5",
	}
	got := BuildCopilotCommand(config)
	want := []string{"copilot", "--allow-all-tools", "--model", "gpt-5"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildCopilotCommandFalseFlag(t *testing.T) {
	config := map[string]interface{}{
		"allow_all_tools": false,
	}
	got := BuildCopilotCommand(config)
	want := []string{"copilot", "--no-allow-all-tools"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildCopilotCommandRepeats(t *testing.T) {
	config := map[string]interface{}{
		"allow_tool": []string{"edit", "write"},
	}
	got := BuildCopilotCommand(config)
	want := []string{"copilot", "--allow-tool", "edit", "--allow-tool", "write"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildCopilotCommandEscapes(t *testing.T) {
	config := map[string]interface{}{
		"prompt": "fix this now",
	}
	got := BuildCopilotCommand(config)
	want := []string{"copilot", "--prompt", "'fix this now'"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
