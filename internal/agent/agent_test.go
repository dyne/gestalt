package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAgentJSONRoundTrip(t *testing.T) {
	input := `{
		"name": "Codex",
		"shell": "/bin/bash",
		"prompt": "coder",
		"onair_string": "READY",
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
	if len(a.Prompts) != 1 || a.Prompts[0] != "coder" {
		t.Fatalf("prompt mismatch: %v", a.Prompts)
	}
	if a.OnAirString != "READY" {
		t.Fatalf("onair_string mismatch: %q", a.OnAirString)
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
	var roundTrip struct {
		Name     string   `json:"name"`
		Shell    string   `json:"shell"`
		Prompt   []string `json:"prompt"`
		OnAir    string   `json:"onair_string"`
		LLMType  string   `json:"llm_type"`
		LLMModel string   `json:"llm_model"`
	}
	if err := json.Unmarshal(out, &roundTrip); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if roundTrip.Name != "Codex" {
		t.Fatalf("roundtrip name mismatch: %q", roundTrip.Name)
	}
	if roundTrip.Shell != "/bin/bash" {
		t.Fatalf("roundtrip shell mismatch: %q", roundTrip.Shell)
	}
	if len(roundTrip.Prompt) != 1 || roundTrip.Prompt[0] != "coder" {
		t.Fatalf("roundtrip prompt mismatch: %v", roundTrip.Prompt)
	}
	if roundTrip.OnAir != "READY" {
		t.Fatalf("roundtrip onair_string mismatch: %q", roundTrip.OnAir)
	}
	if roundTrip.LLMType != "codex" {
		t.Fatalf("roundtrip llm_type mismatch: %q", roundTrip.LLMType)
	}
	if roundTrip.LLMModel != "default" {
		t.Fatalf("roundtrip llm_model mismatch: %q", roundTrip.LLMModel)
	}
}

func TestAgentJSONPromptParsing(t *testing.T) {
	tests := []struct {
		name       string
		promptJSON string
		want       []string
	}{
		{name: "string", promptJSON: `"coder"`, want: []string{"coder"}},
		{name: "array", promptJSON: `["coder","architect"]`, want: []string{"coder", "architect"}},
		{name: "empty string", promptJSON: `""`, want: nil},
		{name: "empty array", promptJSON: `[]`, want: nil},
		{name: "null", promptJSON: `null`, want: nil},
		{name: "missing", promptJSON: "", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input string
			if tt.promptJSON == "" {
				input = `{"name":"Codex","shell":"/bin/bash","llm_type":"codex"}`
			} else {
				input = fmt.Sprintf(`{"name":"Codex","shell":"/bin/bash","prompt":%s,"llm_type":"codex"}`, tt.promptJSON)
			}
			var a Agent
			if err := json.Unmarshal([]byte(input), &a); err != nil {
				t.Fatalf("unmarshal: %v", err)
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
