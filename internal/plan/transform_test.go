package plan

import (
	"path/filepath"
	"testing"
)

func TestTransformDocument(t *testing.T) {
	path := filepath.Join("testdata", "sample.org")
	doc, err := ParseWithOrga(path)
	if err != nil {
		t.Fatalf("ParseWithOrga returned error: %v", err)
	}

	plan := TransformDocument("sample.org", doc)
	if plan.Metadata.Title != "Sample Plan" {
		t.Fatalf("expected title Sample Plan, got %q", plan.Metadata.Title)
	}
	if plan.Metadata.Subtitle != "Example metadata" {
		t.Fatalf("expected subtitle Example metadata, got %q", plan.Metadata.Subtitle)
	}
	if plan.Metadata.Date != "2026-01-26" {
		t.Fatalf("expected date 2026-01-26, got %q", plan.Metadata.Date)
	}
	if plan.L1Count != 2 {
		t.Fatalf("expected L1 count 2, got %d", plan.L1Count)
	}
	if plan.L2Count != 2 {
		t.Fatalf("expected L2 count 2, got %d", plan.L2Count)
	}
	if plan.PriorityA != 0 || plan.PriorityB != 1 || plan.PriorityC != 1 {
		t.Fatalf("expected priority counts A0/B1/C1, got A%d/B%d/C%d", plan.PriorityA, plan.PriorityB, plan.PriorityC)
	}
	if len(plan.Headings) != 2 {
		t.Fatalf("expected 2 L1 headings, got %d", len(plan.Headings))
	}

	first := plan.Headings[0]
	if first.Keyword != "WIP" {
		t.Fatalf("expected first keyword WIP, got %q", first.Keyword)
	}
	if first.Priority != "A" {
		t.Fatalf("expected first priority A, got %q", first.Priority)
	}
	if first.Text != "First L1" {
		t.Fatalf("expected first text First L1, got %q", first.Text)
	}
	expectedBody := "This is the first line of L1 body.\nAnother line for the L1 body."
	if first.Body != expectedBody {
		t.Fatalf("expected first body %q, got %q", expectedBody, first.Body)
	}
	if len(first.Children) != 2 {
		t.Fatalf("expected 2 L2 headings, got %d", len(first.Children))
	}

	child := first.Children[0]
	if child.Keyword != "TODO" {
		t.Fatalf("expected L2 keyword TODO, got %q", child.Keyword)
	}
	if child.Priority != "B" {
		t.Fatalf("expected L2 priority B, got %q", child.Priority)
	}
	if child.Text != "L2 One" {
		t.Fatalf("expected L2 text L2 One, got %q", child.Text)
	}
	if child.Body != "Details for L2 one." {
		t.Fatalf("expected L2 body Details for L2 one., got %q", child.Body)
	}

	childTwo := first.Children[1]
	if childTwo.Keyword != "DONE" {
		t.Fatalf("expected L2 keyword DONE, got %q", childTwo.Keyword)
	}
	if childTwo.Priority != "C" {
		t.Fatalf("expected L2 priority C, got %q", childTwo.Priority)
	}
	if childTwo.Text != "L2 Two" {
		t.Fatalf("expected L2 text L2 Two, got %q", childTwo.Text)
	}
	if childTwo.Body != "More text for L2 two." {
		t.Fatalf("expected L2 body More text for L2 two., got %q", childTwo.Body)
	}

	second := plan.Headings[1]
	if second.Text != "Second L1" {
		t.Fatalf("expected second L1 text Second L1, got %q", second.Text)
	}
	if len(second.Children) != 0 {
		t.Fatalf("expected no L2 children for second L1, got %d", len(second.Children))
	}
}
