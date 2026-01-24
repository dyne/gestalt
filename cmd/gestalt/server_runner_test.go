package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestServerRunnerStopsOnSignal(t *testing.T) {
	runner := &ServerRunner{ShutdownTimeout: 50 * time.Millisecond}
	stop := make(chan os.Signal, 1)
	stop <- os.Interrupt

	serveDone := make(chan struct{})
	var shutdownCalls int32

	server := ManagedServer{
		Name: "backend",
		Serve: func() error {
			<-serveDone
			return http.ErrServerClosed
		},
		Shutdown: func(ctx context.Context) error {
			atomic.AddInt32(&shutdownCalls, 1)
			close(serveDone)
			return nil
		},
	}

	if err := runner.Run(stop, server); err != nil {
		t.Fatalf("expected no server error, got %v", err)
	}
	if atomic.LoadInt32(&shutdownCalls) != 1 {
		t.Fatalf("expected shutdown to be called once")
	}
}

func TestServerRunnerReturnsServerError(t *testing.T) {
	runner := &ServerRunner{ShutdownTimeout: 50 * time.Millisecond}
	stop := make(chan os.Signal)

	var shutdownCalls int32
	server := ManagedServer{
		Name: "backend",
		Serve: func() error {
			return errors.New("boom")
		},
		Shutdown: func(ctx context.Context) error {
			atomic.AddInt32(&shutdownCalls, 1)
			return nil
		},
	}

	serverErr := runner.Run(stop, server)
	if serverErr == nil || serverErr.err == nil {
		t.Fatalf("expected server error")
	}
	if serverErr.name != "backend" {
		t.Fatalf("expected server name backend, got %q", serverErr.name)
	}
	if serverErr.err.Error() != "boom" {
		t.Fatalf("expected error boom, got %v", serverErr.err)
	}
	if atomic.LoadInt32(&shutdownCalls) != 1 {
		t.Fatalf("expected shutdown to be called once")
	}
}
