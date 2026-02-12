package terminal

import (
	"reflect"
	"testing"

	"gestalt/internal/agent"
)

func TestResolveOutputFilterNamesDisabled(t *testing.T) {
	t.Setenv(envTerminalOutputFiltersDisable, "true")
	profile := &agent.Agent{OutputFilters: []string{"ansi-strip"}}
	if got := ResolveOutputFilterNames(profile, agent.AgentInterfaceCLI); got != nil {
		t.Fatalf("expected disabled filters, got %#v", got)
	}
}

func TestResolveOutputFilterNamesEnvOverride(t *testing.T) {
	t.Setenv(envTerminalOutputFilters, "a, b,,c ")
	got := ResolveOutputFilterNames(nil, agent.AgentInterfaceCLI)
	expect := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("expected %v, got %v", expect, got)
	}
}

func TestResolveOutputFilterNamesProfileList(t *testing.T) {
	t.Parallel()

	profile := &agent.Agent{OutputFilters: []string{"x", "y"}}
	got := ResolveOutputFilterNames(profile, agent.AgentInterfaceCLI)
	expect := []string{"x", "y"}
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("expected %v, got %v", expect, got)
	}
}

func TestResolveOutputFilterNamesProfileSingle(t *testing.T) {
	t.Parallel()

	profile := &agent.Agent{OutputFilter: "solo"}
	got := ResolveOutputFilterNames(profile, agent.AgentInterfaceCLI)
	expect := []string{"solo"}
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("expected %v, got %v", expect, got)
	}
}

func TestResolveOutputFilterNamesCodexDefault(t *testing.T) {
	t.Parallel()

	profile := &agent.Agent{CLIType: "codex"}
	got := ResolveOutputFilterNames(profile, agent.AgentInterfaceCLI)
	expect := []string{"scrollback-vt", "ansi-strip", "utf8-guard"}
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("expected %v, got %v", expect, got)
	}
}

func TestResolveOutputFilterNamesCLIDefault(t *testing.T) {
	t.Parallel()

	got := ResolveOutputFilterNames(nil, agent.AgentInterfaceCLI)
	expect := []string{"ansi-strip", "utf8-guard"}
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("expected %v, got %v", expect, got)
	}
}

func TestResolveOutputFilterNamesNonCLISession(t *testing.T) {
	t.Parallel()

	got := ResolveOutputFilterNames(nil, agent.AgentInterfaceMCP)
	if got != nil {
		t.Fatalf("expected nil for non-CLI sessions, got %v", got)
	}
}
