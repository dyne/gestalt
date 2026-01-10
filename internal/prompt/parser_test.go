package prompt

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func newTestParser() *Parser {
	return NewParser(os.DirFS("testdata"), ".")
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
