package main

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

func stubCommandDeps() commandDeps {
	return commandDeps{
		Stdout:            io.Discard,
		Stderr:            io.Discard,
		RunServer:         func(args []string) int { return 0 },
		RunValidateSkill:  func(args []string) int { return 0 },
		RunValidateConfig: func(args []string) int { return 0 },
		RunCompletion:     func(args []string, out io.Writer, errOut io.Writer) int { return 0 },
		RunExtractConfig:  func() int { return 0 },
	}
}

func TestResolveCommandValidateSkill(t *testing.T) {
	deps := stubCommandDeps()
	var gotArgs []string
	deps.RunValidateSkill = func(args []string) int {
		gotArgs = append([]string(nil), args...)
		return 7
	}

	cmd, cmdArgs := resolveCommand([]string{"validate-skill", "path/to/skill"}, deps)
	if code := cmd.Run(cmdArgs); code != 7 {
		t.Fatalf("expected code 7, got %d", code)
	}
	if !reflect.DeepEqual(gotArgs, []string{"path/to/skill"}) {
		t.Fatalf("expected args to be forwarded, got %v", gotArgs)
	}
}

func TestResolveCommandValidateConfig(t *testing.T) {
	deps := stubCommandDeps()
	var gotArgs []string
	deps.RunValidateConfig = func(args []string) int {
		gotArgs = append([]string(nil), args...)
		return 5
	}

	cmd, cmdArgs := resolveCommand([]string{"config", "validate", "--agents-dir", "agents"}, deps)
	if code := cmd.Run(cmdArgs); code != 5 {
		t.Fatalf("expected code 5, got %d", code)
	}
	if !reflect.DeepEqual(gotArgs, []string{"--agents-dir", "agents"}) {
		t.Fatalf("expected args to be forwarded, got %v", gotArgs)
	}
}

func TestResolveCommandCompletion(t *testing.T) {
	deps := stubCommandDeps()
	var gotArgs []string
	var gotOut io.Writer
	var gotErr io.Writer
	deps.Stdout = &bytes.Buffer{}
	deps.Stderr = &bytes.Buffer{}
	deps.RunCompletion = func(args []string, out io.Writer, errOut io.Writer) int {
		gotArgs = append([]string(nil), args...)
		gotOut = out
		gotErr = errOut
		return 3
	}

	cmd, cmdArgs := resolveCommand([]string{"completion", "bash"}, deps)
	if code := cmd.Run(cmdArgs); code != 3 {
		t.Fatalf("expected code 3, got %d", code)
	}
	if !reflect.DeepEqual(gotArgs, []string{"bash"}) {
		t.Fatalf("expected args to be forwarded, got %v", gotArgs)
	}
	if gotOut != deps.Stdout || gotErr != deps.Stderr {
		t.Fatalf("expected completion to use provided writers")
	}
}

func TestResolveCommandExtractConfig(t *testing.T) {
	deps := stubCommandDeps()
	called := false
	deps.RunExtractConfig = func() int {
		called = true
		return 2
	}

	cmd, cmdArgs := resolveCommand([]string{"--extract-config"}, deps)
	if code := cmd.Run(cmdArgs); code != 2 {
		t.Fatalf("expected code 2, got %d", code)
	}
	if !called {
		t.Fatalf("expected extract config to run")
	}
}

func TestResolveCommandServerDefault(t *testing.T) {
	deps := stubCommandDeps()
	var gotArgs []string
	deps.RunServer = func(args []string) int {
		gotArgs = append([]string(nil), args...)
		return 4
	}

	cmd, cmdArgs := resolveCommand([]string{"--port", "8080"}, deps)
	if code := cmd.Run(cmdArgs); code != 4 {
		t.Fatalf("expected code 4, got %d", code)
	}
	if !reflect.DeepEqual(gotArgs, []string{"--port", "8080"}) {
		t.Fatalf("expected args to be forwarded, got %v", gotArgs)
	}
}
