package main

import "testing"

func TestParseEndpointPort(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantPort int
		wantOK   bool
	}{
		{name: "plain port", endpoint: "4318", wantPort: 4318, wantOK: true},
		{name: "host port", endpoint: "127.0.0.1:4318", wantPort: 4318, wantOK: true},
		{name: "missing host", endpoint: ":4318", wantPort: 4318, wantOK: true},
		{name: "empty", endpoint: "", wantPort: 0, wantOK: false},
		{name: "invalid", endpoint: "localhost:notaport", wantPort: 0, wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			port, ok := parseEndpointPort(test.endpoint)
			if ok != test.wantOK {
				t.Fatalf("expected ok=%v, got %v (port=%d)", test.wantOK, ok, port)
			}
			if ok && port != test.wantPort {
				t.Fatalf("expected port %d, got %d", test.wantPort, port)
			}
		})
	}
}
