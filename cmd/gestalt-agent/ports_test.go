package main

import "testing"

func TestParsePortFromAddress(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"7233", 7233},
		{"localhost:7233", 7233},
		{":4318", 4318},
		{"http://localhost:4318", 4318},
		{"127.0.0.1:9998", 9998},
	}
	for _, tc := range cases {
		port, ok := parsePortFromAddress(tc.input)
		if !ok {
			t.Fatalf("expected port for %q", tc.input)
		}
		if port != tc.want {
			t.Fatalf("expected port %d for %q, got %d", tc.want, tc.input, port)
		}
	}
}

func TestDefaultPortResolverDefaults(t *testing.T) {
	resolver := defaultPortResolver()
	if port, ok := resolver.Get("frontend"); !ok || port != defaultFrontendPort {
		t.Fatalf("expected frontend port %d, got %d (ok=%v)", defaultFrontendPort, port, ok)
	}
	if port, ok := resolver.Get("backend"); !ok || port != defaultFrontendPort {
		t.Fatalf("expected backend port %d, got %d (ok=%v)", defaultFrontendPort, port, ok)
	}
	if port, ok := resolver.Get("temporal"); !ok || port != defaultTemporalPort {
		t.Fatalf("expected temporal port %d, got %d (ok=%v)", defaultTemporalPort, port, ok)
	}
	if port, ok := resolver.Get("otel"); !ok || port != defaultOtelPort {
		t.Fatalf("expected otel port %d, got %d (ok=%v)", defaultOtelPort, port, ok)
	}
}

func TestDefaultPortResolverOverrides(t *testing.T) {
	t.Setenv("GESTALT_PORT", "6000")
	t.Setenv("GESTALT_BACKEND_PORT", "7000")
	t.Setenv("GESTALT_TEMPORAL_HOST", "localhost:8000")
	t.Setenv("GESTALT_OTEL_HTTP_ENDPOINT", "http://127.0.0.1:9000")

	resolver := defaultPortResolver()
	if port, ok := resolver.Get("frontend"); !ok || port != 6000 {
		t.Fatalf("expected frontend port 6000, got %d (ok=%v)", port, ok)
	}
	if port, ok := resolver.Get("backend"); !ok || port != 7000 {
		t.Fatalf("expected backend port 7000, got %d (ok=%v)", port, ok)
	}
	if port, ok := resolver.Get("temporal"); !ok || port != 8000 {
		t.Fatalf("expected temporal port 8000, got %d (ok=%v)", port, ok)
	}
	if port, ok := resolver.Get("otel"); !ok || port != 9000 {
		t.Fatalf("expected otel port 9000, got %d (ok=%v)", port, ok)
	}
}
