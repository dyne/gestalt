package main

import (
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"gestalt/internal/ports"
)

const (
	defaultFrontendPort = 57417
	defaultTemporalPort = 7233
	defaultOtelPort     = 4318
)

func defaultPortResolver() ports.PortResolver {
	frontendPort := envPort("GESTALT_PORT", defaultFrontendPort)
	backendPort := envPort("GESTALT_BACKEND_PORT", 0)
	if backendPort == 0 {
		backendPort = frontendPort
	}
	temporalPort := envPortFromAddress("GESTALT_TEMPORAL_HOST", defaultTemporalPort)
	otelPort := envPortFromAddress("GESTALT_OTEL_HTTP_ENDPOINT", defaultOtelPort)

	registry := ports.NewPortRegistry()
	registry.Set("frontend", frontendPort)
	registry.Set("backend", backendPort)
	registry.Set("temporal", temporalPort)
	registry.Set("otel", otelPort)
	return registry
}

func envPort(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	if port, ok := parsePortNumber(value); ok {
		return port
	}
	return fallback
}

func envPortFromAddress(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	if port, ok := parsePortFromAddress(value); ok {
		return port
	}
	return fallback
}

func parsePortFromAddress(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if port, ok := parsePortNumber(value); ok {
		return port, true
	}
	if strings.Contains(value, "://") {
		if parsed, err := url.Parse(value); err == nil {
			if port, ok := parsePortNumber(parsed.Port()); ok {
				return port, true
			}
		}
		return 0, false
	}
	if strings.Contains(value, ":") {
		if _, port, err := net.SplitHostPort(value); err == nil {
			return parsePortNumber(port)
		}
	}
	if strings.Contains(value, "/") {
		if parsed, err := url.Parse("http://" + value); err == nil {
			if port, ok := parsePortNumber(parsed.Port()); ok {
				return port, true
			}
		}
	}
	return 0, false
}

func parsePortNumber(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	port, err := strconv.Atoi(value)
	if err != nil || port <= 0 || port > 65535 {
		return 0, false
	}
	return port, true
}
