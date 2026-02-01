package main

import (
	"context"
	"net/http"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"gestalt/internal/logging"
)

func TestShutdownWatcherStopsDaemonsOnSignal(t *testing.T) {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	signalCh := make(chan os.Signal, 1)
	stopCtx, stopCancel := context.WithCancel(context.Background())
	defer stopCancel()

	stopped := make(chan struct{})
	startShutdownWatcher(shutdownCtx, func() {
		select {
		case <-stopped:
		default:
			close(stopped)
		}
	})

	go func() {
		<-signalCh
		shutdownCancel()
		stopCancel()
	}()

	serveStop := make(chan struct{})
	runner := &ServerRunner{ShutdownTimeout: 50 * time.Millisecond}
	done := make(chan struct{})
	go func() {
		runner.Run(stopCtx, ManagedServer{
			Name: "test",
			Serve: func() error {
				<-serveStop
				return http.ErrServerClosed
			},
			Shutdown: func(ctx context.Context) error {
				close(serveStop)
				return nil
			},
		})
		close(done)
	}()

	signalCh <- os.Interrupt

	select {
	case <-stopped:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected daemon stop on shutdown")
	}
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected runner to stop")
	}
}

func TestWatchShutdownSignalsCancelsOnceAndLogsRepeat(t *testing.T) {
	logBuffer := logging.NewLogBuffer(logging.DefaultBufferSize)
	logger := logging.NewLogger(logBuffer, logging.LevelInfo)
	signalCh := make(chan os.Signal, 3)

	var cancelCalls int32
	stop := watchShutdownSignals(logger, func() {
		atomic.AddInt32(&cancelCalls, 1)
	}, signalCh)
	defer stop()

	signalCh <- os.Interrupt
	signalCh <- os.Interrupt
	signalCh <- syscall.SIGTERM

	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		if atomic.LoadInt32(&cancelCalls) == 1 && len(logBuffer.List()) >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected shutdown cancel and logs before timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}

	entries := logBuffer.List()
	firstCount := 0
	ignoreCount := 0
	for _, entry := range entries {
		switch entry.Message {
		case "shutdown signal received":
			firstCount++
		case "shutdown already in progress; ignoring signal":
			ignoreCount++
		}
	}

	if firstCount != 1 {
		t.Fatalf("expected 1 shutdown signal log, got %d", firstCount)
	}
	if ignoreCount != 1 {
		t.Fatalf("expected 1 ignore log, got %d", ignoreCount)
	}
	if atomic.LoadInt32(&cancelCalls) != 1 {
		t.Fatalf("expected shutdown cancel once, got %d", cancelCalls)
	}
}
