package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentJSONRoundTrip(t *testing.T) {
	input := `{
		"name": "Codex",
		"shell": "/bin/bash",
		"prompt_file": "config/prompts/codex.txt",
		"llm_type": "codex",
		"llm_model": "default"
	}`

	var a Agent
	if err := json.Unmarshal([]byte(input), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.Name != "Codex" {
		t.Fatalf("name mismatch: %q", a.Name)
	}
	if a.Shell != "/bin/bash" {
		t.Fatalf("shell mismatch: %q", a.Shell)
	}
	if a.PromptFile != "config/prompts/codex.txt" {
		t.Fatalf("prompt_file mismatch: %q", a.PromptFile)
	}
	if a.LLMType != "codex" {
		t.Fatalf("llm_type mismatch: %q", a.LLMType)
	}
	if a.LLMModel != "default" {
		t.Fatalf("llm_model mismatch: %q", a.LLMModel)
	}

	out, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var roundTrip map[string]string
	if err := json.Unmarshal(out, &roundTrip); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if roundTrip["name"] != "Codex" {
		t.Fatalf("roundtrip name mismatch: %q", roundTrip["name"])
	}
	if roundTrip["shell"] != "/bin/bash" {
		t.Fatalf("roundtrip shell mismatch: %q", roundTrip["shell"])
	}
	if roundTrip["prompt_file"] != "config/prompts/codex.txt" {
		t.Fatalf("roundtrip prompt_file mismatch: %q", roundTrip["prompt_file"])
	}
	if roundTrip["llm_type"] != "codex" {
		t.Fatalf("roundtrip llm_type mismatch: %q", roundTrip["llm_type"])
	}
	if roundTrip["llm_model"] != "default" {
		t.Fatalf("roundtrip llm_model mismatch: %q", roundTrip["llm_model"])
	}
}

func TestAgentValidate(t *testing.T) {
	tests := []struct {
		name    string
		agent   Agent
		wantErr string
	}{
		{
			name: "valid",
			agent: Agent{
				Name:    "Codex",
				Shell:   "/bin/bash",
				LLMType: "codex",
			},
		},
		{
			name: "missing name",
			agent: Agent{
				Name:    " ",
				Shell:   "/bin/bash",
				LLMType: "codex",
			},
			wantErr: "agent name is required",
		},
		{
			name: "missing shell",
			agent: Agent{
				Name:    "Codex",
				Shell:   " ",
				LLMType: "codex",
			},
			wantErr: "agent shell is required",
		},
		{
			name: "missing llm_type",
			agent: Agent{
				Name:  "Codex",
				Shell: "/bin/bash",
			},
			wantErr: "agent llm_type is required",
		},
		{
			name: "invalid llm_type",
			agent: Agent{
				Name:    "Codex",
				Shell:   "/bin/bash",
				LLMType: "other",
			},
			wantErr: "agent llm_type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.agent.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
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
