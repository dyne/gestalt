package terminal

import (
	"strings"
	"testing"
)

func TestStripANSIRemovesSequencesAndControlCodes(t *testing.T) {
	input := "hello\x1b[31mred\x1b[0m\x07world"
	got := StripANSI(input)
	if got != "helloredworld" {
		t.Fatalf("expected %q, got %q", "helloredworld", got)
	}
}

func TestStripANSIPreservesWhitespaceControls(t *testing.T) {
	input := "line1\tline2\nline3\rline4\x00"
	got := StripANSI(input)
	if got != "line1\tline2\nline3\rline4" {
		t.Fatalf("expected whitespace preserved, got %q", got)
	}
}

func TestStripRepeatedChars(t *testing.T) {
	input := "start-----end"
	got := StripRepeatedChars(input, 3)
	if got != "start-end" {
		t.Fatalf("expected collapsed run, got %q", got)
	}
	if StripRepeatedChars("short--", 3) != "short--" {
		t.Fatalf("expected short runs unchanged")
	}
}

func TestStripRepeatedCharsUTF8(t *testing.T) {
	input := "cafééé"
	got := StripRepeatedChars(input, 3)
	if got != "café" {
		t.Fatalf("expected UTF-8 run collapsed, got %q", got)
	}
}

func TestFilterTerminalOutputMixed(t *testing.T) {
	input := "ok\x1b[32mgreen\x1b[0m\n-----\x00done"
	got := FilterTerminalOutput(input)
	if strings.Contains(got, "\x1b") {
		t.Fatalf("expected ANSI stripped, got %q", got)
	}
	if strings.Contains(got, "-----") {
		t.Fatalf("expected repeated chars collapsed, got %q", got)
	}
	if !strings.Contains(got, "green") || !strings.Contains(got, "done") {
		t.Fatalf("expected content preserved, got %q", got)
	}
}
