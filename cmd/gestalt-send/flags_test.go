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
	cfg, err := parseArgs([]string{"agent"}, &stderr)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if cfg.URL != "http://127.0.0.1:57417" {
		t.Fatalf("expected default url %q, got %q", "http://127.0.0.1:57417", cfg.URL)
	}
	if cfg.Token != "" {
		t.Fatalf("expected empty token, got %q", cfg.Token)
	}
	if cfg.AgentRef != "agent" {
		t.Fatalf("expected agent ref agent, got %q", cfg.AgentRef)
	}
	if cfg.Start {
		t.Fatalf("expected start false")
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
		"--start",
		"--verbose",
		"--debug",
		"agent",
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
	if !cfg.Start {
		t.Fatalf("expected start true")
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
	if !strings.Contains(stderr.String(), "Usage: gestalt-send") {
		t.Fatalf("expected help output, got %q", stderr.String())
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

func TestParseArgsSessionIDWithoutAgent(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"--session-id", "s-1"}, &stderr)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if cfg.SessionID != "s-1 1" {
		t.Fatalf("expected normalized session id s-1 1, got %q", cfg.SessionID)
	}
	if cfg.AgentRef != "" {
		t.Fatalf("expected empty agent ref, got %q", cfg.AgentRef)
	}
}
