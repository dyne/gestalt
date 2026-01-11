package config

import (
	"strings"
	"testing"

	"gestalt/internal/logging"
	"gestalt/internal/version"
)

func TestCheckVersionCompatibilityMajorMismatch(t *testing.T) {
	installed := version.VersionInfo{Major: 1, Minor: 0, Patch: 0}
	current := version.VersionInfo{Major: 2, Minor: 0, Patch: 0}

	err := CheckVersionCompatibility(installed, current, nil)
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

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	if err := CheckVersionCompatibility(installed, current, logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasVersionLog(buffer.List(), logging.LevelWarning, minorMismatchMessage) {
		t.Fatalf("expected warning log")
	}
}

func TestCheckVersionCompatibilityPatchMismatchLogsInfo(t *testing.T) {
	installed := version.VersionInfo{Major: 1, Minor: 2, Patch: 1}
	current := version.VersionInfo{Major: 1, Minor: 2, Patch: 3}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	if err := CheckVersionCompatibility(installed, current, logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasVersionLog(buffer.List(), logging.LevelInfo, "Config updated from 1.2.1 to 1.2.3") {
		t.Fatalf("expected info log")
	}
}

func TestCheckVersionCompatibilityNoMismatch(t *testing.T) {
	installed := version.VersionInfo{Major: 1, Minor: 2, Patch: 3}
	current := version.VersionInfo{Major: 1, Minor: 2, Patch: 3}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	if err := CheckVersionCompatibility(installed, current, logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(buffer.List()) != 0 {
		t.Fatalf("expected no log output")
	}
}

func hasVersionLog(entries []logging.LogEntry, level logging.Level, message string) bool {
	for _, entry := range entries {
		if entry.Level == level && strings.Contains(entry.Message, message) {
			return true
		}
	}
	return false
}
