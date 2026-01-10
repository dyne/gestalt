package prompt

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func newTestParser() *Parser {
	return NewParser(os.DirFS("testdata"), ".", "testdata")
}

func TestRenderPlainText(t *testing.T) {
	parser := newTestParser()
	result, err := parser.Render("simple")
	if err != nil {
		t.Fatalf("render simple: %v", err)
	}
	expectedContent := "simple prompt line\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"simple.txt"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderTemplateSingleInclude(t *testing.T) {
	parser := newTestParser()
	result, err := parser.Render("basic")
	if err != nil {
		t.Fatalf("render basic: %v", err)
	}
	expectedContent := "Header line\ncommon fragment line\nFooter line\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"basic.tmpl", "common-fragment.txt"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderTemplateMultipleIncludes(t *testing.T) {
	parser := newTestParser()
	result, err := parser.Render("multi")
	if err != nil {
		t.Fatalf("render multi: %v", err)
	}
	expectedContent := "Start line\ncommon fragment line\nextra fragment line\nEnd line\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"multi.tmpl", "common-fragment.txt", "extra.txt"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderNestedIncludes(t *testing.T) {
	parser := newTestParser()
	result, err := parser.Render("nested")
	if err != nil {
		t.Fatalf("render nested: %v", err)
	}
	expectedContent := "Top line\nInner start\ncommon fragment line\nInner end\nBottom line\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"nested.tmpl", "inner.tmpl", "common-fragment.txt"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderMissingInclude(t *testing.T) {
	parser := newTestParser()
	result, err := parser.Render("missing-include")
	if err != nil {
		t.Fatalf("render missing-include: %v", err)
	}
	expectedContent := "Before\nAfter\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"missing-include.tmpl"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderIncludeWithoutExtension(t *testing.T) {
	parser := newTestParser()
	result, err := parser.Render("include-noext")
	if err != nil {
		t.Fatalf("render include-noext: %v", err)
	}
	expectedContent := "Head line\ncommon fragment line\nTail line\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"include-noext.tmpl", "common-fragment.txt"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderCycleDetection(t *testing.T) {
	parser := newTestParser()
	_, err := parser.Render("cycle-a")
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderTemplateNoIncludes(t *testing.T) {
	parser := newTestParser()
	result, err := parser.Render("template-only")
	if err != nil {
		t.Fatalf("render template-only: %v", err)
	}
	expectedContent := "Template only line\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"template-only.tmpl"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderMissingPrompt(t *testing.T) {
	parser := newTestParser()
	_, err := parser.Render("missing-top")
	if err == nil {
		t.Fatal("expected missing prompt error")
	}
	if !strings.Contains(err.Error(), "missing-top") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderIncludeWrongExtension(t *testing.T) {
	parser := newTestParser()
	result, err := parser.Render("include-wrong-ext")
	if err != nil {
		t.Fatalf("render include-wrong-ext: %v", err)
	}
	expectedContent := "Top line\nBottom line\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"include-wrong-ext.tmpl"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderIncludeWhitespace(t *testing.T) {
	parser := newTestParser()
	result, err := parser.Render("include-whitespace")
	if err != nil {
		t.Fatalf("render include-whitespace: %v", err)
	}
	expectedContent := "Start line\ncommon fragment line\nEnd line\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"include-whitespace.tmpl", "common-fragment.txt"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderIncludeEdges(t *testing.T) {
	parser := newTestParser()
	result, err := parser.Render("include-edges")
	if err != nil {
		t.Fatalf("render include-edges: %v", err)
	}
	expectedContent := "common fragment line\nMiddle line\nextra fragment line\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"include-edges.tmpl", "common-fragment.txt", "extra.txt"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderIncludeRepeat(t *testing.T) {
	parser := newTestParser()
	result, err := parser.Render("include-repeat")
	if err != nil {
		t.Fatalf("render include-repeat: %v", err)
	}
	expectedContent := "Start line\ncommon fragment line\ncommon fragment line\nEnd line\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"include-repeat.tmpl", "common-fragment.txt", "common-fragment.txt"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderIncludeEmptyLines(t *testing.T) {
	parser := newTestParser()
	result, err := parser.Render("include-empty-lines")
	if err != nil {
		t.Fatalf("render include-empty-lines: %v", err)
	}
	expectedContent := "Top line\n\ncommon fragment line\n\nBottom line\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"include-empty-lines.tmpl", "common-fragment.txt"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderDirectCycle(t *testing.T) {
	parser := newTestParser()
	_, err := parser.Render("cycle-direct")
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
	if !strings.Contains(err.Error(), "cycle detected") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "cycle-direct.tmpl") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderDeepCycle(t *testing.T) {
	parser := newTestParser()
	_, err := parser.Render("cycle-deep-a")
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
	if !strings.Contains(err.Error(), "cycle detected") && !strings.Contains(err.Error(), "depth exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "cycle-deep-a.tmpl") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderDepthLimit(t *testing.T) {
	parser := newTestParser()
	_, err := parser.Render("depth-1")
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	if !strings.Contains(err.Error(), "depth exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}
