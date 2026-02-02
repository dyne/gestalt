package main

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestParseArgsAgentID(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"coder"}, &stderr)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if cfg.AgentID != "coder" {
		t.Fatalf("expected agent id coder, got %q", cfg.AgentID)
	}
}

func TestParseArgsTomlSuffix(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"Coder.TOML"}, &stderr)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if cfg.AgentID != "Coder" {
		t.Fatalf("expected agent id Coder, got %q", cfg.AgentID)
	}
}

func TestParseArgsRejectsPath(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseArgs([]string{"config/coder.toml"}, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "path") {
		t.Fatalf("expected path error, got %v", err)
	}
}

func TestParseArgsRejectsBackslashPath(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseArgs([]string{`config\coder.toml`}, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "path") {
		t.Fatalf("expected path error, got %v", err)
	}
}

func TestParseArgsHelp(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseArgs([]string{"--help"}, &stderr)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected ErrHelp, got %v", err)
	}
	if !strings.Contains(stderr.String(), "Usage: gestalt-agent") {
		t.Fatalf("expected help output, got %q", stderr.String())
	}
}

func TestParseArgsVersion(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"--version"}, &stderr)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if !cfg.ShowVersion {
		t.Fatalf("expected version flag")
	}
}

func TestParseArgsMissingAgent(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseArgs([]string{}, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseArgsDryRun(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"--dryrun", "coder"}, &stderr)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if !cfg.DryRun {
		t.Fatalf("expected dryrun")
	}
}
