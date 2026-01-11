package main

import (
	"strings"
	"testing"
)

func sampleLines() []string {
	return []string{
		"* TODO [#B] Feature A",
		"  Notes for A",
		"** TODO [#A] Task A1",
		"*** NOTE Subtask A1.1",
		"** DONE Task A2",
		"* WIP [#A] Feature B",
		"** WIP Task B1",
		"*** TODO Subtask B1.1",
		"** TODO Task B2",
		"* DONE Feature C",
	}
}

func TestParseHeading(t *testing.T) {
	line := "** TODO [#A] Example"
	heading, ok := parseHeading(line, 4)
	if !ok {
		t.Fatal("expected heading to parse")
	}
	if heading.Level != 2 {
		t.Fatalf("expected level 2, got %d", heading.Level)
	}
	if heading.Status != "TODO" {
		t.Fatalf("expected TODO status, got %q", heading.Status)
	}
	if heading.Priority != "A" {
		t.Fatalf("expected priority A, got %q", heading.Priority)
	}
	if heading.Title != "Example" {
		t.Fatalf("expected title Example, got %q", heading.Title)
	}
}

func TestCurrent(t *testing.T) {
	plan := parsePlan(sampleLines())
	results, warnings := current(plan)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Feature B" {
		t.Fatalf("unexpected L1 title: %s", results[0].Title)
	}
	if results[1].Title != "Task B1" {
		t.Fatalf("unexpected L2 title: %s", results[1].Title)
	}
}

func TestSetStatus(t *testing.T) {
	lines, err := setStatus(sampleLines(), 2, "Task B2", "DONE")
	if err != nil {
		t.Fatalf("setStatus error: %v", err)
	}
	if got := lines[8]; got != "** DONE Task B2" {
		t.Fatalf("unexpected line: %s", got)
	}
}

func TestCompleteL1(t *testing.T) {
	lines, err := completeL1(sampleLines(), "Feature B")
	if err != nil {
		t.Fatalf("completeL1 error: %v", err)
	}
	if got := lines[5]; got != "* DONE [#A] Feature B" {
		t.Fatalf("unexpected L1 line: %s", got)
	}
	if got := lines[6]; got != "** Task B1" {
		t.Fatalf("unexpected L2 line: %s", got)
	}
	if got := lines[8]; got != "** Task B2" {
		t.Fatalf("unexpected L2 line: %s", got)
	}
}

func TestInsertL2AfterSubtree(t *testing.T) {
	lines := []string{
		"* TODO Feature",
		"** TODO Task 1",
		"*** NOTE Subtask",
		"** TODO Task 2",
		"*** NOTE Deep",
		"* DONE Later",
	}
	updated, err := insertL2(lines, "Feature", "Task 3", "B")
	if err != nil {
		t.Fatalf("insertL2 error: %v", err)
	}
	want := "** TODO [#B] Task 3"
	if updated[5] != want {
		t.Fatalf("expected insert after subtree, got: %s", updated[5])
	}
}

func TestShowL1(t *testing.T) {
	lines := sampleLines()
	section, err := showL1(lines, "Feature B")
	if err != nil {
		t.Fatalf("showL1 error: %v", err)
	}
	joined := strings.Join(section, "\n")
	if !strings.Contains(joined, "* WIP [#A] Feature B") {
		t.Fatalf("missing L1 heading in section")
	}
	if strings.Contains(joined, "* DONE Feature C") {
		t.Fatalf("section leaked into next L1")
	}
}

func TestLint(t *testing.T) {
	lines := []string{
		"* WIP Feature A",
		"** WIP Task A",
		"* WIP Feature B",
		"** WIP Task B",
	}
	plan := parsePlan(lines)
	results, violations := lint(plan)
	if !violations {
		t.Fatalf("expected violations")
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
}
