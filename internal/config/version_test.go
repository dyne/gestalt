package config

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"gestalt/internal/version"
)

func TestCheckVersionCompatibilityMajorMismatch(t *testing.T) {
	installed := version.VersionInfo{Major: 1, Minor: 0, Patch: 0}
	current := version.VersionInfo{Major: 2, Minor: 0, Patch: 0}

	err := CheckVersionCompatibility(installed, current)
	if err == nil {
		t.Fatalf("expected error for major mismatch")
	}
	if !strings.Contains(err.Error(), majorMismatchMessage) {
		t.Fatalf("expected error to include guidance message, got %q", err.Error())
	}
}

func TestCheckVersionCompatibilityMinorMismatchLogsWarning(t *testing.T) {
	installed := version.VersionInfo{Major: 1, Minor: 1, Patch: 0}
	current := version.VersionInfo{Major: 1, Minor: 2, Patch: 0}

	output := captureLogOutput(t, func() {
		if err := CheckVersionCompatibility(installed, current); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, minorMismatchMessage) {
		t.Fatalf("expected warning log, got %q", output)
	}
}

func TestCheckVersionCompatibilityPatchMismatchLogsInfo(t *testing.T) {
	installed := version.VersionInfo{Major: 1, Minor: 2, Patch: 1}
	current := version.VersionInfo{Major: 1, Minor: 2, Patch: 3}

	output := captureLogOutput(t, func() {
		if err := CheckVersionCompatibility(installed, current); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "Config updated from 1.2.1 to 1.2.3") {
		t.Fatalf("expected info log, got %q", output)
	}
}

func TestCheckVersionCompatibilityNoMismatch(t *testing.T) {
	installed := version.VersionInfo{Major: 1, Minor: 2, Patch: 3}
	current := version.VersionInfo{Major: 1, Minor: 2, Patch: 3}

	output := captureLogOutput(t, func() {
		if err := CheckVersionCompatibility(installed, current); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if output != "" {
		t.Fatalf("expected no log output, got %q", output)
	}
}

func captureLogOutput(t *testing.T, fn func()) string {
	t.Helper()
	var buffer bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&buffer)
	log.SetFlags(0)

	fn()

	log.SetOutput(previousOutput)
	log.SetFlags(previousFlags)
	return strings.TrimSpace(buffer.String())
}
