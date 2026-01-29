package main

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestShutdownWatcherStopsDaemonsOnSignal(t *testing.T) {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	signalCh := make(chan os.Signal, 1)
	stopSignals := make(chan os.Signal, 1)

	stopped := make(chan struct{})
	startShutdownWatcher(shutdownCtx, func() {
		select {
		case <-stopped:
		default:
			close(stopped)
		}
	})

	go func() {
		sig := <-signalCh
		shutdownCancel()
		stopSignals <- sig
	}()

	serveStop := make(chan struct{})
	runner := &ServerRunner{ShutdownTimeout: 50 * time.Millisecond}
	done := make(chan struct{})
	go func() {
		runner.Run(stopSignals, ManagedServer{
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
