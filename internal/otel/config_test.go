package otel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCollectorConfigWritesFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "collector.yaml")
	dataPath := filepath.Join(tempDir, "otel", "otel.json")

	err := WriteCollectorConfig(configPath, dataPath, "127.0.0.1:4317", "127.0.0.1:4318")
	if err != nil {
		t.Fatalf("WriteCollectorConfig failed: %v", err)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "endpoint: \"127.0.0.1:4317\"") {
		t.Fatalf("expected grpc endpoint in config: %s", text)
	}
	if !strings.Contains(text, "endpoint: \"127.0.0.1:4318\"") {
		t.Fatalf("expected http endpoint in config: %s", text)
	}
	if !strings.Contains(text, "path: \""+dataPath+"\"") {
		t.Fatalf("expected data path in config: %s", text)
	}
}

func TestOptionsFromEnvDefaults(t *testing.T) {
	opts := OptionsFromEnv("state")
	if !opts.Enabled {
		t.Fatalf("expected Enabled true by default")
	}
	if opts.GRPCEndpoint != defaultGRPCEndpoint {
		t.Fatalf("expected default grpc endpoint, got %q", opts.GRPCEndpoint)
	}
	if opts.HTTPEndpoint != defaultHTTPEndpoint {
		t.Fatalf("expected default http endpoint, got %q", opts.HTTPEndpoint)
	}
	if opts.DataDir != filepath.Join("state", "otel") {
		t.Fatalf("expected data dir under state root, got %q", opts.DataDir)
	}
	if opts.ConfigPath != filepath.Join(opts.DataDir, "collector.yaml") {
		t.Fatalf("expected config path under data dir, got %q", opts.ConfigPath)
	}
}

func TestOptionsFromEnvOverrides(t *testing.T) {
	t.Setenv("GESTALT_OTEL_ENABLED", "false")
	t.Setenv("GESTALT_OTEL_COLLECTOR", "/tmp/otelcol")
	t.Setenv("GESTALT_OTEL_CONFIG", "/tmp/collector.yaml")
	t.Setenv("GESTALT_OTEL_DATA_DIR", "/tmp/otel")
	t.Setenv("GESTALT_OTEL_GRPC_ENDPOINT", "127.0.0.1:9999")
	t.Setenv("GESTALT_OTEL_HTTP_ENDPOINT", "127.0.0.1:9998")

	opts := OptionsFromEnv("state")
	if opts.Enabled {
		t.Fatalf("expected Enabled false with env override")
	}
	if opts.BinaryPath != "/tmp/otelcol" {
		t.Fatalf("expected BinaryPath override, got %q", opts.BinaryPath)
	}
	if opts.ConfigPath != "/tmp/collector.yaml" {
		t.Fatalf("expected ConfigPath override, got %q", opts.ConfigPath)
	}
	if opts.DataDir != "/tmp/otel" {
		t.Fatalf("expected DataDir override, got %q", opts.DataDir)
	}
	if opts.GRPCEndpoint != "127.0.0.1:9999" {
		t.Fatalf("expected GRPCEndpoint override, got %q", opts.GRPCEndpoint)
	}
	if opts.HTTPEndpoint != "127.0.0.1:9998" {
		t.Fatalf("expected HTTPEndpoint override, got %q", opts.HTTPEndpoint)
	}
}
