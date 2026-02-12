package launchspec

import "testing"

func TestNormalizeLaunchSpecDefaults(t *testing.T) {
	spec := LaunchSpec{
		SessionID:   "  session-1 ",
		Interface:   " mcp ",
		PromptFiles: []string{" one ", "", "one"},
		GUIModules:  []string{"console", " ", "console"},
	}

	normalized := NormalizeLaunchSpec(spec)

	if normalized.SessionID != "session-1" {
		t.Fatalf("expected trimmed session id, got %q", normalized.SessionID)
	}
	if normalized.Interface != "mcp" {
		t.Fatalf("expected trimmed interface, got %q", normalized.Interface)
	}
	if len(normalized.PromptFiles) != 1 || normalized.PromptFiles[0] != "one" {
		t.Fatalf("expected normalized prompt files, got %#v", normalized.PromptFiles)
	}
	if len(normalized.GUIModules) != 1 || normalized.GUIModules[0] != "console" {
		t.Fatalf("expected normalized gui modules, got %#v", normalized.GUIModules)
	}
	if normalized.PromptInjection.Mode != PromptInjectionNone {
		t.Fatalf("expected default prompt injection mode, got %q", normalized.PromptInjection.Mode)
	}
}

func TestNormalizePromptInjectionDefaultsPacing(t *testing.T) {
	spec := PromptInjectionSpec{
		Mode: PromptInjectionStdin,
	}

	normalized := NormalizePromptInjection(spec)
	expected := DefaultPromptInjectionPacing()

	if normalized.Pacing != expected {
		t.Fatalf("expected default pacing, got %#v", normalized.Pacing)
	}
}
