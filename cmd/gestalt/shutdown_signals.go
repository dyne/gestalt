package main

import (
	"context"
	"os"
	"sync/atomic"

	"gestalt/internal/logging"
)

func watchShutdownSignals(logger *logging.Logger, shutdownCancel context.CancelFunc, signalCh <-chan os.Signal) func() {
	if signalCh == nil {
		return func() {}
	}

	done := make(chan struct{})
	var shutdownStarted atomic.Bool
	var loggedRepeat atomic.Bool

	go func() {
		for {
			select {
			case <-done:
				return
			case sig, ok := <-signalCh:
				if !ok {
					return
				}
				if shutdownStarted.CompareAndSwap(false, true) {
					if logger != nil {
						fields := map[string]string{}
						if sig != nil {
							fields["signal"] = sig.String()
						}
						logger.Info("shutdown signal received", fields)
					}
					if shutdownCancel != nil {
						shutdownCancel()
					}
					continue
				}
				if loggedRepeat.CompareAndSwap(false, true) && logger != nil {
					fields := map[string]string{}
					if sig != nil {
						fields["signal"] = sig.String()
					}
					logger.Info("shutdown already in progress; ignoring signal", fields)
				}
			}
		}
	}()

	return func() {
		close(done)
	}
}
