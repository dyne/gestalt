package prompt

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"gestalt/internal/ports"
)

type mockPortResolver struct {
	ports map[string]int
}

func (resolver *mockPortResolver) Get(service string) (int, bool) {
	port, found := resolver.ports[service]
	return port, found
}

func newTestParserWithResolver(resolver ports.PortResolver) *Parser {
	return NewParser(os.DirFS("testdata"), ".", "testdata", resolver)
}

func newTestParser() *Parser {
	return newTestParserWithResolver(nil)
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

func TestRenderTextIncludeDirective(t *testing.T) {
	parser := newTestParser()
	result, err := parser.Render("text-include")
	if err != nil {
		t.Fatalf("render text-include: %v", err)
	}
	expectedContent := "Text start\ncommon fragment line\nText end\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"text-include.txt", "common-fragment.txt"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderMarkdownPortDirective(t *testing.T) {
	resolver := &mockPortResolver{
		ports: map[string]int{
			"backend": 8080,
		},
	}
	parser := newTestParserWithResolver(resolver)
	result, err := parser.Render("markdown-port")
	if err != nil {
		t.Fatalf("render markdown-port: %v", err)
	}
	expectedContent := "Before\n8080\nAfter\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"markdown-port.md"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderTextPortDirective(t *testing.T) {
	resolver := &mockPortResolver{
		ports: map[string]int{
			"backend": 8080,
		},
	}
	parser := newTestParserWithResolver(resolver)
	result, err := parser.Render("text-port")
	if err != nil {
		t.Fatalf("render text-port: %v", err)
	}
	expectedContent := "Port:\n8080\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"text-port.txt"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderInlineDirective(t *testing.T) {
	parser := newTestParser()
	result, err := parser.Render("inline-directive")
	if err != nil {
		t.Fatalf("render inline-directive: %v", err)
	}
	expectedContent := "Inline common fragment line\n directive\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"inline-directive.txt", "common-fragment.txt"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderInlineSessionID(t *testing.T) {
	parser := newTestParser()
	ctx := RenderContext{SessionID: "session-42"}
	result, err := parser.RenderWithContext("session-id-inline", ctx)
	if err != nil {
		t.Fatalf("render session-id-inline: %v", err)
	}
	expectedContent := "Session=session-42\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"session-id-inline.tmpl"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderSessionIDLineDirectiveWithoutContext(t *testing.T) {
	parser := newTestParser()
	result, err := parser.RenderWithContext("session-id-line", RenderContext{})
	if err != nil {
		t.Fatalf("render session-id-line: %v", err)
	}
	expectedContent := "Start\nEnd\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"session-id-line.tmpl"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderInlinePortDirective(t *testing.T) {
	resolver := &mockPortResolver{
		ports: map[string]int{
			"backend": 8080,
		},
	}
	parser := newTestParserWithResolver(resolver)
	result, err := parser.Render("inline-port")
	if err != nil {
		t.Fatalf("render inline-port: %v", err)
	}
	expectedContent := "http://localhost:8080/api\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"inline-port.tmpl"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderInlineEscapedDirective(t *testing.T) {
	parser := newTestParser()
	ctx := RenderContext{SessionID: "session-42"}
	result, err := parser.RenderWithContext("inline-escape", ctx)
	if err != nil {
		t.Fatalf("render inline-escape: %v", err)
	}
	expectedContent := "Literal {{session id}}\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"inline-escape.tmpl"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderInlineMultipleDirectives(t *testing.T) {
	resolver := &mockPortResolver{
		ports: map[string]int{
			"backend": 8080,
			"otel":    4318,
		},
	}
	parser := newTestParserWithResolver(resolver)
	ctx := RenderContext{SessionID: "session-42"}
	result, err := parser.RenderWithContext("inline-multi", ctx)
	if err != nil {
		t.Fatalf("render inline-multi: %v", err)
	}
	expectedContent := "Ports=8080/4318 Session=session-42\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"inline-multi.tmpl"}
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
	expectedContent := "Start line\ncommon fragment line\nEnd line\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"include-repeat.tmpl", "common-fragment.txt"}
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

func TestRenderPortDirectiveWithResolver(t *testing.T) {
	resolver := &mockPortResolver{
		ports: map[string]int{
			"backend": 8080,
			"otel":    4318,
		},
	}
	parser := newTestParserWithResolver(resolver)

	result, err := parser.Render("port")
	if err != nil {
		t.Fatalf("render port: %v", err)
	}
	expectedContent := "Before\n8080\nBetween\n4318\nAfter\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"port.tmpl"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderPortDirectiveWithoutResolver(t *testing.T) {
	parser := newTestParserWithResolver(nil)

	result, err := parser.Render("port")
	if err != nil {
		t.Fatalf("render port: %v", err)
	}
	expectedContent := "Before\nBetween\nAfter\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
}

func TestRenderPortDirectiveUnknownService(t *testing.T) {
	resolver := &mockPortResolver{
		ports: map[string]int{
			"backend": 8080,
		},
	}
	parser := newTestParserWithResolver(resolver)

	result, err := parser.Render("port")
	if err != nil {
		t.Fatalf("render port: %v", err)
	}
	expectedContent := "Before\n8080\nBetween\nAfter\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
}

func TestRenderIncludeWithPortDirective(t *testing.T) {
	resolver := &mockPortResolver{
		ports: map[string]int{
			"backend": 8080,
		},
	}
	parser := newTestParserWithResolver(resolver)

	result, err := parser.Render("include-port")
	if err != nil {
		t.Fatalf("render include-port: %v", err)
	}
	expectedContent := "Header\n8080\nFooter\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"include-port.tmpl", "fragment-port.tmpl"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderMixedIncludeAndPortDirective(t *testing.T) {
	resolver := &mockPortResolver{
		ports: map[string]int{
			"backend": 8080,
		},
	}
	parser := newTestParserWithResolver(resolver)

	result, err := parser.Render("mixed-port")
	if err != nil {
		t.Fatalf("render mixed-port: %v", err)
	}
	expectedContent := "Alpha\n8080\ncommon fragment line\nOmega\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"mixed-port.tmpl", "common-fragment.txt"}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestParsePortDirective(t *testing.T) {
	longService := strings.Repeat("a", 33)
	tests := []struct {
		name        string
		line        string
		wantService string
		wantOK      bool
	}{
		{
			name:        "basic",
			line:        "{{port backend}}\n",
			wantService: "backend",
			wantOK:      true,
		},
		{
			name:        "whitespace and case",
			line:        "  {{ port BACKEND }}  ",
			wantService: "backend",
			wantOK:      true,
		},
		{
			name:   "missing service",
			line:   "{{port}}",
			wantOK: false,
		},
		{
			name:   "extra fields",
			line:   "{{port backend extra}}",
			wantOK: false,
		},
		{
			name:   "different directive",
			line:   "{{include backend}}",
			wantOK: false,
		},
		{
			name:   "too long",
			line:   "{{port " + longService + "}}",
			wantOK: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, ok := parsePortDirective(test.line)
			if ok != test.wantOK {
				t.Fatalf("expected ok=%v, got %v (service=%q)", test.wantOK, ok, service)
			}
			if ok && service != test.wantService {
				t.Fatalf("expected service %q, got %q", test.wantService, service)
			}
		})
	}
}
