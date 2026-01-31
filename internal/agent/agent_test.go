package agent

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestAgentTOMLDecode(t *testing.T) {
	input := `
name = "Codex"
shell = "/bin/bash"
prompt = "coder"
skills = ["git-workflows", "code-review"]
onair_string = "READY"
cli_type = "codex"
codex_mode = "tui"
llm_model = "default"
`

	var a Agent
	if _, err := toml.Decode(input, &a); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if a.Name != "Codex" {
		t.Fatalf("name mismatch: %q", a.Name)
	}
	if a.Shell != "/bin/bash" {
		t.Fatalf("shell mismatch: %q", a.Shell)
	}
	if len(a.Prompts) != 1 || a.Prompts[0] != "coder" {
		t.Fatalf("prompt mismatch: %v", a.Prompts)
	}
	if len(a.Skills) != 2 || a.Skills[0] != "git-workflows" || a.Skills[1] != "code-review" {
		t.Fatalf("skills mismatch: %v", a.Skills)
	}
	if a.OnAirString != "READY" {
		t.Fatalf("onair_string mismatch: %q", a.OnAirString)
	}
	if a.CLIType != "codex" {
		t.Fatalf("cli_type mismatch: %q", a.CLIType)
	}
	if a.CodexMode != "tui" {
		t.Fatalf("codex_mode mismatch: %q", a.CodexMode)
	}
	if a.LLMModel != "default" {
		t.Fatalf("llm_model mismatch: %q", a.LLMModel)
	}
}

func TestAgentSingletonParsing(t *testing.T) {
	withSingleton, err := loadAgentFromBytes("config/agents/coder.toml", []byte(`
name = "Coder"
shell = "/bin/bash"
singleton = false
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if withSingleton.Singleton == nil || *withSingleton.Singleton {
		t.Fatalf("expected singleton=false")
	}

	defaultSingleton, err := loadAgentFromBytes("config/agents/coder.toml", []byte(`
name = "Coder"
shell = "/bin/bash"
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultSingleton.Singleton == nil || !*defaultSingleton.Singleton {
		t.Fatalf("expected singleton default true")
	}
}

func TestAgentTOMLPromptParsing(t *testing.T) {
	tests := []struct {
		name       string
		promptTOML string
		want       []string
	}{
		{name: "string", promptTOML: `prompt = "coder"`, want: []string{"coder"}},
		{name: "array", promptTOML: `prompt = ["coder", "architect"]`, want: []string{"coder", "architect"}},
		{name: "empty string", promptTOML: `prompt = ""`, want: nil},
		{name: "empty array", promptTOML: `prompt = []`, want: nil},
		{name: "missing", promptTOML: "", want: nil},
		{name: "trim blanks", promptTOML: `prompt = ["", " coder "]`, want: []string{"coder"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "name = \"Codex\"\nshell = \"/bin/bash\"\n"
			if tt.promptTOML != "" {
				input += tt.promptTOML + "\n"
			}
			var a Agent
			if _, err := toml.Decode(input, &a); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(a.Prompts) != len(tt.want) {
				t.Fatalf("prompt length mismatch: %v", a.Prompts)
			}
			for i, got := range a.Prompts {
				if got != tt.want[i] {
					t.Fatalf("prompt %d mismatch: %q", i, got)
				}
			}
		})
	}
}

func TestAgentValidate(t *testing.T) {
	tests := []struct {
		name       string
		agent      Agent
		wantErr    string
		wantShell  string
		checkShell bool
	}{
		{
			name: "valid shell",
			agent: Agent{
				Name:    "Codex",
				Shell:   "/bin/bash",
				CLIType: "codex",
			},
			wantShell:  "/bin/bash",
			checkShell: true,
		},
		{
			name: "cli_config builds shell",
			agent: Agent{
				Name:    "Codex",
				CLIType: "codex",
				CLIConfig: map[string]interface{}{
					"model": "o3",
				},
			},
			wantShell:  "",
			checkShell: true,
		},
		{
			name: "missing cli_type with cli_config",
			agent: Agent{
				Name: "Codex",
				CLIConfig: map[string]interface{}{
					"model": "o3",
				},
			},
			wantErr: "agent cli_type is required",
		},
		{
			name: "missing name",
			agent: Agent{
				Name:    " ",
				Shell:   "/bin/bash",
				CLIType: "codex",
			},
			wantErr: "agent name is required",
		},
		{
			name: "missing shell",
			agent: Agent{
				Name:    "Codex",
				Shell:   " ",
				CLIType: "codex",
			},
			wantErr: "agent shell is required",
		},
		{
			name: "invalid codex_mode",
			agent: Agent{
				Name:      "Codex",
				Shell:     "/bin/bash",
				CLIType:   "codex",
				CodexMode: "bogus",
			},
			wantErr: "codex_mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := tt.agent
			err := agent.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tt.checkShell && strings.TrimSpace(agent.Shell) != strings.TrimSpace(tt.wantShell) {
					t.Fatalf("shell mismatch: %q", agent.Shell)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestAgentCodexModeDefault(t *testing.T) {
	agent := Agent{
		Name:    "Codex",
		Shell:   "/bin/bash",
		CLIType: "codex",
	}
	if err := agent.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.CodexMode != CodexModeMCPServer {
		t.Fatalf("expected codex_mode %q, got %q", CodexModeMCPServer, agent.CodexMode)
	}
}

func TestAgentNormalizeShell(t *testing.T) {
	agent := Agent{
		Name:    "Codex",
		CLIType: "codex",
		CLIConfig: map[string]interface{}{
			"model": "o3",
		},
	}
	if err := agent.NormalizeShell(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(agent.Shell) == "" {
		t.Fatalf("expected shell to be set")
	}
}

func TestAgentNormalizeShellMissingType(t *testing.T) {
	agent := Agent{
		Name: "Codex",
		CLIConfig: map[string]interface{}{
			"model": "o3",
		},
	}
	if err := agent.NormalizeShell(); err == nil {
		t.Fatalf("expected error")
	}
}
