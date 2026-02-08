package shellgen

import (
	"reflect"
	"testing"
)

func TestBuildOllamaCommandSimple(t *testing.T) {
	config := map[string]interface{}{
		"model": "llama2",
	}
	got := BuildOllamaCommand(config)
	want := []string{"ollama", "run", "llama2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildOllamaCommandWithHost(t *testing.T) {
	config := map[string]interface{}{
		"model": "llama2",
		"host":  "http://remote:11434",
	}
	got := BuildOllamaCommand(config)
	want := []string{"env", "OLLAMA_HOST=http://remote:11434", "ollama", "run", "llama2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildOllamaCommandWithFlags(t *testing.T) {
	config := map[string]interface{}{
		"model":    "llama2",
		"insecure": true,
		"format":   "json",
	}
	got := BuildOllamaCommand(config)
	want := []string{"ollama", "run", "llama2", "--format", "json", "--insecure"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildOllamaCommandEscapes(t *testing.T) {
	config := map[string]interface{}{
		"model": "llama2 7b",
	}
	got := BuildOllamaCommand(config)
	want := []string{"ollama", "run", "'llama2 7b'"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
