package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"gestalt/internal/otel"
)

func TestOTelUILogIngestWritesCollectorFile(t *testing.T) {
	grpcPort, httpPort, err := reserveOTelPorts()
	if err != nil {
		t.Skipf("skipping otel ingest test (ports unavailable): %v", err)
	}

	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, ".gestalt", "otel")
	options := otel.Options{
		Enabled:      true,
		DataDir:      dataDir,
		ConfigPath:   filepath.Join(dataDir, "collector.yaml"),
		GRPCEndpoint: net.JoinHostPort("127.0.0.1", strconv.Itoa(grpcPort)),
		HTTPEndpoint: net.JoinHostPort("127.0.0.1", strconv.Itoa(httpPort)),
	}

	collector, err := otel.StartCollector(options)
	if err != nil {
		if err == otel.ErrCollectorNotFound {
			t.Skip("otel collector binary not available")
		}
		t.Fatalf("start collector: %v", err)
	}
	defer func() {
		_ = otel.StopCollectorWithTimeout(collector, 5*time.Second)
	}()

	sdkShutdown, err := otel.SetupSDK(context.Background(), otel.SDKOptions{
		Enabled:        true,
		HTTPEndpoint:   options.HTTPEndpoint,
		ServiceName:    "gestalt-test",
		ServiceVersion: "test",
	})
	if err != nil {
		t.Fatalf("SetupSDK error: %v", err)
	}

	payload := otel.LoadUILogFixture(t)
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	rest := &RestHandler{}
	handler := restHandler("", nil, rest.handleOTelLogs)
	req := httptest.NewRequest(http.MethodPost, "/api/otel/logs", bytes.NewReader(body))
	resp := httptest.NewRecorder()
	handler(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", resp.Code)
	}

	if sdkShutdown != nil {
		_ = sdkShutdown(context.Background())
		sdkShutdown = nil
	}

	dataPath := filepath.Join(dataDir, "otel.json")
	if !waitForNonEmptyFile(dataPath, 3*time.Second) {
		t.Fatalf("expected otel.json to have data")
	}
}

func reserveOTelPorts() (int, int, error) {
	grpcPort, err := reservePort()
	if err != nil {
		return 0, 0, err
	}
	httpPort, err := reservePort()
	if err != nil {
		return 0, 0, err
	}
	if httpPort == grpcPort {
		return reserveOTelPorts()
	}
	return grpcPort, httpPort, nil
}

func reservePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener addr")
	}
	return addr.Port, nil
}

func waitForNonEmptyFile(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := os.Stat(path)
		if err == nil && info.Size() > 0 {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}
