package main

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestParseArgsDefaults(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"s-1"}, &stderr)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if cfg.URL != "http://127.0.0.1:57417" {
		t.Fatalf("expected default url %q, got %q", "http://127.0.0.1:57417", cfg.URL)
	}
	if cfg.Token != "" {
		t.Fatalf("expected empty token, got %q", cfg.Token)
	}
	if cfg.SessionRef != "s-1" {
		t.Fatalf("expected session ref s-1, got %q", cfg.SessionRef)
	}
	if cfg.Verbose {
		t.Fatalf("expected verbose false")
	}
	if cfg.Debug {
		t.Fatalf("expected debug false")
	}
}

func TestParseArgsFlagOverridesEnv(t *testing.T) {
	t.Setenv("GESTALT_TOKEN", "secret")
	var stderr bytes.Buffer

	cfg, err := parseArgs([]string{
		"--host", "override",
		"--port", "4210",
		"--token", "override-token",
		"--verbose",
		"--debug",
		"session-9",
	}, &stderr)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if cfg.URL != "http://override:4210" {
		t.Fatalf("expected override url, got %q", cfg.URL)
	}
	if cfg.Token != "override-token" {
		t.Fatalf("expected override token, got %q", cfg.Token)
	}
	if !cfg.Verbose {
		t.Fatalf("expected verbose true")
	}
	if !cfg.Debug {
		t.Fatalf("expected debug true")
	}
}

func TestParseArgsHelp(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseArgs([]string{"--help"}, &stderr)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected ErrHelp, got %v", err)
	}
	help := stderr.String()
	if !strings.Contains(help, "Usage: gestalt-send [options] <session-ref>") {
		t.Fatalf("expected positional usage, got %q", help)
	}
	if strings.Contains(help, "--session-id") {
		t.Fatalf("did not expect legacy --session-id in help")
	}
}

func TestParseArgsHelpShort(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseArgs([]string{"-h"}, &stderr)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected ErrHelp, got %v", err)
	}
	if !strings.Contains(stderr.String(), "Usage: gestalt-send") {
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
		t.Fatalf("expected version flag to be set")
	}
}

func TestParseArgsVersionShort(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"-v"}, &stderr)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if !cfg.ShowVersion {
		t.Fatalf("expected version flag to be set")
	}
}

func TestParseArgsInvalidFlag(t *testing.T) {
	var stderr bytes.Buffer
	if _, err := parseArgs([]string{"--host"}, &stderr); err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseArgsRejectsMissingPositionalArgument(t *testing.T) {
	var stderr bytes.Buffer
	if _, err := parseArgs([]string{}, &stderr); err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseArgsRejectsMultiplePositionalArguments(t *testing.T) {
	var stderr bytes.Buffer
	if _, err := parseArgs([]string{"s-1", "s-2"}, &stderr); err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseArgsRejectsLegacySessionIDFlag(t *testing.T) {
	var stderr bytes.Buffer
	if _, err := parseArgs([]string{"--session-id", "s-1"}, &stderr); err == nil {
		t.Fatalf("expected error for legacy --session-id")
	}
}

func TestParseArgsSessionRef(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"  s-1  "}, &stderr)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if cfg.SessionRef != "s-1" {
		t.Fatalf("expected trimmed session ref s-1, got %q", cfg.SessionRef)
	}
}

func TestCompletionScriptsDoNotExposeLegacySessionIDFlag(t *testing.T) {
	if strings.Contains(bashCompletionScript, "--session-id") {
		t.Fatalf("bash completion must not include --session-id")
	}
	if strings.Contains(zshCompletionScript, "--session-id") {
		t.Fatalf("zsh completion must not include --session-id")
	}
}
