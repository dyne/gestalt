package main

import (
	"net"
	"strconv"
	"testing"
)

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

func TestIsPortAvailable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	if isPortAvailable(port) {
		t.Fatalf("expected port %d to be unavailable", port)
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	if !isPortAvailable(port) {
		t.Fatalf("expected port %d to be available", port)
	}
}

func TestResolveOTelPortsUsesDefaultsWhenAvailable(t *testing.T) {
	grpcPort, httpPort := findAdjacentPorts(t)

	gotGRPC, gotHTTP, err := resolveOTelPorts(grpcPort, httpPort)
	if err != nil {
		t.Fatalf("resolveOTelPorts failed: %v", err)
	}
	if gotGRPC != grpcPort || gotHTTP != httpPort {
		t.Fatalf("expected ports %d/%d, got %d/%d", grpcPort, httpPort, gotGRPC, gotHTTP)
	}
}

func TestResolveOTelPortsRandomizesWhenDefaultOccupied(t *testing.T) {
	grpcPort, httpPort := findAdjacentPorts(t)

	grpcListener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", itoa(grpcPort)))
	if err != nil {
		t.Fatalf("listen grpc: %v", err)
	}
	httpListener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", itoa(httpPort)))
	if err != nil {
		_ = grpcListener.Close()
		t.Fatalf("listen http: %v", err)
	}
	defer func() {
		_ = grpcListener.Close()
		_ = httpListener.Close()
	}()

	gotGRPC, gotHTTP, err := resolveOTelPorts(grpcPort, httpPort)
	if err != nil {
		t.Fatalf("resolveOTelPorts failed: %v", err)
	}
	if gotGRPC == grpcPort && gotHTTP == httpPort {
		t.Fatalf("expected randomized ports, got defaults %d/%d", gotGRPC, gotHTTP)
	}
	if gotHTTP != gotGRPC+1 {
		t.Fatalf("expected adjacent ports, got %d/%d", gotGRPC, gotHTTP)
	}
	if !isPortAvailable(gotGRPC) || !isPortAvailable(gotHTTP) {
		t.Fatalf("expected available ports, got %d/%d", gotGRPC, gotHTTP)
	}
}

func findAdjacentPorts(t *testing.T) (int, int) {
	t.Helper()
	for attempt := 0; attempt < 50; attempt++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			continue
		}
		port := listener.Addr().(*net.TCPAddr).Port
		if err := listener.Close(); err != nil {
			continue
		}
		if port <= 0 || port >= 65535 {
			continue
		}
		if !isPortAvailable(port) || !isPortAvailable(port+1) {
			continue
		}
		return port, port + 1
	}
	t.Fatal("failed to find adjacent free ports")
	return 0, 0
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
