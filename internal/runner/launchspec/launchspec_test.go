package launchspec

import "testing"

func TestNormalizeLaunchSpecDefaults(t *testing.T) {
	spec := LaunchSpec{
		SessionID:   "  session-1 ",
		Interface:   " mcp ",
		PromptFiles: []string{" one ", "", "one"},
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

func TestBuildArgvCodexIncludesCommand(t *testing.T) {
	argv := BuildArgv("codex", map[string]interface{}{"model": "o3"}, "prompt")
	if len(argv) == 0 || argv[0] != "codex" {
		t.Fatalf("expected codex argv prefix, got %#v", argv)
	}
	found := false
	for i := 0; i < len(argv); i++ {
		if argv[i] == "-c" && i+1 < len(argv) && argv[i+1] == "developer_instructions=prompt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected developer_instructions in argv, got %#v", argv)
	}
}

func TestBuildArgvUnknownType(t *testing.T) {
	if argv := BuildArgv("unknown", nil, "prompt"); argv != nil {
		t.Fatalf("expected nil argv, got %#v", argv)
	}
}
