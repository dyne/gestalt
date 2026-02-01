package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"gestalt/internal/logging"
)

type ManagedServer struct {
	Name     string
	Serve    func() error
	Shutdown func(context.Context) error
}

type ServerRunner struct {
	Logger          *logging.Logger
	ShutdownTimeout time.Duration
}

type serverError struct {
	name string
	err  error
}

func (runner *ServerRunner) Run(stop context.Context, servers ...ManagedServer) *serverError {
	started := 0
	errorsChan := make(chan serverError, len(servers))
	for _, server := range servers {
		server := server
		if server.Serve == nil {
			continue
		}
		started++
		go func() {
			errorsChan <- serverError{name: server.Name, err: server.Serve()}
		}()
	}

	if started == 0 {
		return nil
	}

	var initialError *serverError
	select {
	case err := <-errorsChan:
		initialError = &err
	case <-stop.Done():
	}

	runner.logServerError(initialError)

	timeout := runner.ShutdownTimeout
	if timeout <= 0 {
		timeout = httpServerShutdownTimeout
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for _, server := range servers {
		if server.Shutdown == nil {
			continue
		}
		if err := server.Shutdown(shutdownContext); err != nil && runner.Logger != nil {
			message := fmt.Sprintf("%s server shutdown failed", server.Name)
			runner.Logger.Warn(message, map[string]string{
				"error": err.Error(),
			})
		}
	}

	runner.drainServerErrors(errorsChan, started, initialError != nil, timeout)
	return initialError
}

func (runner *ServerRunner) logServerError(serverErr *serverError) {
	if runner == nil || runner.Logger == nil || serverErr == nil || serverErr.err == nil {
		return
	}
	if errors.Is(serverErr.err, http.ErrServerClosed) {
		return
	}
	runner.Logger.Error("http server stopped", map[string]string{
		"server": serverErr.name,
		"error":  serverErr.err.Error(),
	})
}

func (runner *ServerRunner) drainServerErrors(errorsChan <-chan serverError, total int, initialLogged bool, timeout time.Duration) {
	if total <= 0 {
		return
	}
	pending := total
	if initialLogged {
		pending--
	}
	for i := 0; i < pending; i++ {
		select {
		case err := <-errorsChan:
			runner.logServerError(&err)
		case <-time.After(timeout):
			return
		}
	}
}
