package main

import (
	"errors"
	"net"
	"strconv"
	"testing"
)

func TestShutdownPhaseOrder(t *testing.T) {
	phases := buildShutdownPhases(nil, nil, nil, nil, nil, nil, nil, nil)
	names := make([]string, 0, len(phases))
	for _, phase := range phases {
		names = append(names, phase.name)
	}
	want := []string{
		"flow-bridge",
		"sessions",
		"otel-sdk",
		"otel-collector",
		"otel-fallback",
		"fs-watcher",
		"event-bus",
		"process-registry",
	}
	if len(names) != len(want) {
		t.Fatalf("expected %d phases, got %d", len(want), len(names))
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("expected phase %d to be %q, got %q", i, want[i], names[i])
		}
	}
}

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

type fakeTmuxSessionTerminator struct {
	hasSession bool
	hasErr     error
	killErr    error
	killed     []string
	checked    []string
}

func (f *fakeTmuxSessionTerminator) HasSession(name string) (bool, error) {
	f.checked = append(f.checked, name)
	if f.hasErr != nil {
		return false, f.hasErr
	}
	return f.hasSession, nil
}

func (f *fakeTmuxSessionTerminator) KillSession(name string) error {
	f.killed = append(f.killed, name)
	return f.killErr
}

func TestStopAgentsTmuxSessionSkipsWhenMissing(t *testing.T) {
	previousFactory := newTmuxSessionTerminator
	previousSessionName := workdirTmuxSessionName
	fakeClient := &fakeTmuxSessionTerminator{hasSession: false}
	newTmuxSessionTerminator = func() tmuxSessionTerminator { return fakeClient }
	workdirTmuxSessionName = func() (string, error) { return "Gestalt repo", nil }
	t.Cleanup(func() {
		newTmuxSessionTerminator = previousFactory
		workdirTmuxSessionName = previousSessionName
	})

	if err := stopAgentsTmuxSession(nil); err != nil {
		t.Fatalf("stopAgentsTmuxSession returned error: %v", err)
	}
	if len(fakeClient.checked) != 1 || fakeClient.checked[0] != "Gestalt repo" {
		t.Fatalf("expected has-session check for Gestalt repo, got %v", fakeClient.checked)
	}
	if len(fakeClient.killed) != 0 {
		t.Fatalf("expected no kill call, got %v", fakeClient.killed)
	}
}

func TestStopAgentsTmuxSessionKillsWhenPresent(t *testing.T) {
	previousFactory := newTmuxSessionTerminator
	previousSessionName := workdirTmuxSessionName
	fakeClient := &fakeTmuxSessionTerminator{hasSession: true}
	newTmuxSessionTerminator = func() tmuxSessionTerminator { return fakeClient }
	workdirTmuxSessionName = func() (string, error) { return "Gestalt repo", nil }
	t.Cleanup(func() {
		newTmuxSessionTerminator = previousFactory
		workdirTmuxSessionName = previousSessionName
	})

	if err := stopAgentsTmuxSession(nil); err != nil {
		t.Fatalf("stopAgentsTmuxSession returned error: %v", err)
	}
	if len(fakeClient.killed) != 1 || fakeClient.killed[0] != "Gestalt repo" {
		t.Fatalf("expected one kill-session call for Gestalt repo, got %v", fakeClient.killed)
	}
}

func TestStopAgentsTmuxSessionReturnsHasSessionError(t *testing.T) {
	previousFactory := newTmuxSessionTerminator
	previousSessionName := workdirTmuxSessionName
	expectedErr := errors.New("tmux has-session failed")
	fakeClient := &fakeTmuxSessionTerminator{hasErr: expectedErr}
	newTmuxSessionTerminator = func() tmuxSessionTerminator { return fakeClient }
	workdirTmuxSessionName = func() (string, error) { return "Gestalt repo", nil }
	t.Cleanup(func() {
		newTmuxSessionTerminator = previousFactory
		workdirTmuxSessionName = previousSessionName
	})

	err := stopAgentsTmuxSession(nil)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected has-session error %v, got %v", expectedErr, err)
	}
}

func TestStopAgentsTmuxSessionReturnsKillError(t *testing.T) {
	previousFactory := newTmuxSessionTerminator
	previousSessionName := workdirTmuxSessionName
	expectedErr := errors.New("tmux kill failed")
	fakeClient := &fakeTmuxSessionTerminator{hasSession: true, killErr: expectedErr}
	newTmuxSessionTerminator = func() tmuxSessionTerminator { return fakeClient }
	workdirTmuxSessionName = func() (string, error) { return "Gestalt repo", nil }
	t.Cleanup(func() {
		newTmuxSessionTerminator = previousFactory
		workdirTmuxSessionName = previousSessionName
	})

	err := stopAgentsTmuxSession(nil)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected kill-session error %v, got %v", expectedErr, err)
	}
}
