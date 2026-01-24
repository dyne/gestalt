package otel

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultGRPCEndpoint = "127.0.0.1:4317"
	defaultHTTPEndpoint = "127.0.0.1:4318"
)

func WriteCollectorConfig(path, dataPath, grpcEndpoint, httpEndpoint, remoteEndpoint string, remoteInsecure bool) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("collector config path is required")
	}
	if strings.TrimSpace(dataPath) == "" {
		return errors.New("collector data path is required")
	}
	grpcEndpoint = strings.TrimSpace(grpcEndpoint)
	httpEndpoint = strings.TrimSpace(httpEndpoint)
	if grpcEndpoint == "" {
		grpcEndpoint = defaultGRPCEndpoint
	}
	if httpEndpoint == "" {
		httpEndpoint = defaultHTTPEndpoint
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dataPath), 0o755); err != nil {
		return err
	}

	config := buildCollectorConfig(grpcEndpoint, httpEndpoint, dataPath, remoteEndpoint, remoteInsecure)
	return os.WriteFile(path, []byte(config), 0o644)
}

func buildCollectorConfig(grpcEndpoint, httpEndpoint, dataPath, remoteEndpoint string, remoteInsecure bool) string {
	grpcValue := strconv.Quote(grpcEndpoint)
	httpValue := strconv.Quote(httpEndpoint)
	pathValue := strconv.Quote(dataPath)
	builder := strings.Builder{}
	builder.WriteString("receivers:\n")
	builder.WriteString("  otlp:\n")
	builder.WriteString("    protocols:\n")
	builder.WriteString("      grpc:\n")
	builder.WriteString("        endpoint: ")
	builder.WriteString(grpcValue)
	builder.WriteString("\n")
	builder.WriteString("      http:\n")
	builder.WriteString("        endpoint: ")
	builder.WriteString(httpValue)
	builder.WriteString("\n")
	builder.WriteString("\nprocessors:\n")
	builder.WriteString("  batch: {}\n")
	builder.WriteString("\nexporters:\n")
	builder.WriteString("  file:\n")
	builder.WriteString("    path: ")
	builder.WriteString(pathValue)
	builder.WriteString("\n")
	builder.WriteString("    format: json\n")
	builder.WriteString("    append: true\n")
	builder.WriteString("    create_directory: true\n")
	if strings.TrimSpace(remoteEndpoint) != "" {
		builder.WriteString("  otlp:\n")
		builder.WriteString("    endpoint: ")
		builder.WriteString(strconv.Quote(strings.TrimSpace(remoteEndpoint)))
		builder.WriteString("\n")
		if remoteInsecure {
			builder.WriteString("    tls:\n")
			builder.WriteString("      insecure: true\n")
		}
	}
	builder.WriteString("\nservice:\n")
	builder.WriteString("  pipelines:\n")
	builder.WriteString("    logs:\n")
	builder.WriteString("      receivers: [otlp]\n")
	builder.WriteString("      processors: [batch]\n")
	builder.WriteString("      exporters: [file")
	if strings.TrimSpace(remoteEndpoint) != "" {
		builder.WriteString(", otlp")
	}
	builder.WriteString("]\n")
	builder.WriteString("    metrics:\n")
	builder.WriteString("      receivers: [otlp]\n")
	builder.WriteString("      processors: [batch]\n")
	builder.WriteString("      exporters: [file")
	if strings.TrimSpace(remoteEndpoint) != "" {
		builder.WriteString(", otlp")
	}
	builder.WriteString("]\n")
	builder.WriteString("    traces:\n")
	builder.WriteString("      receivers: [otlp]\n")
	builder.WriteString("      processors: [batch]\n")
	builder.WriteString("      exporters: [file")
	if strings.TrimSpace(remoteEndpoint) != "" {
		builder.WriteString(", otlp")
	}
	builder.WriteString("]\n")
	return builder.String()
}
