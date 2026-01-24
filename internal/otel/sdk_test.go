package otel

import "testing"

func TestParseResourceAttributes(t *testing.T) {
	attrs := parseResourceAttributes("env=dev, team = core ,invalid,=skip")
	if attrs["env"] != "dev" {
		t.Fatalf("expected env=dev, got %q", attrs["env"])
	}
	if attrs["team"] != "core" {
		t.Fatalf("expected team=core, got %q", attrs["team"])
	}
	if _, ok := attrs["invalid"]; ok {
		t.Fatalf("expected invalid attribute to be skipped")
	}
	if _, ok := attrs[""]; ok {
		t.Fatalf("expected empty key to be skipped")
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	cases := map[string]string{
		"http://127.0.0.1:4318":  "127.0.0.1:4318",
		"https://localhost:4318": "localhost:4318",
		"127.0.0.1:4318/":        "127.0.0.1:4318",
		"":                       "",
	}
	for input, expected := range cases {
		if got := normalizeEndpoint(input); got != expected {
			t.Fatalf("normalizeEndpoint(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestSDKOptionsFromEnv(t *testing.T) {
	t.Setenv("GESTALT_OTEL_SDK_ENABLED", "false")
	t.Setenv("GESTALT_OTEL_SERVICE_NAME", "gestalt-test")
	t.Setenv("GESTALT_OTEL_RESOURCE_ATTRIBUTES", "env=staging")
	t.Setenv("GESTALT_OTEL_HTTP_ENDPOINT", "127.0.0.1:9998")

	opts := SDKOptionsFromEnv("state")
	if opts.Enabled {
		t.Fatalf("expected Enabled false")
	}
	if opts.ServiceName != "gestalt-test" {
		t.Fatalf("expected service name override, got %q", opts.ServiceName)
	}
	if opts.HTTPEndpoint != "127.0.0.1:9998" {
		t.Fatalf("expected http endpoint override, got %q", opts.HTTPEndpoint)
	}
	if opts.ResourceAttributes["env"] != "staging" {
		t.Fatalf("expected resource attribute env=staging, got %v", opts.ResourceAttributes)
	}
}
