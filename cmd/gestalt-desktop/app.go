package main

import (
	"context"
	"net/http"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/terminal"
	"gestalt/internal/version"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx       context.Context
	serverURL string
	manager   *terminal.Manager
	server    *http.Server
	logger    *logging.Logger
}

func NewApp(url string, manager *terminal.Manager, server *http.Server, logger *logging.Logger) *App {
	return &App{
		serverURL: url,
		manager:   manager,
		server:    server,
		logger:    logger,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	if a.server == nil {
		return
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.server.Shutdown(shutdownContext); err != nil && a.logger != nil {
		a.logger.Warn("desktop server shutdown failed", map[string]string{
			"error": err.Error(),
		})
	}
}

func (a *App) beforeClose(ctx context.Context) bool {
	return false
}

func (a *App) GetServerURL() string {
	return a.serverURL
}

func (a *App) GetVersion() string {
	return version.Version
}

func (a *App) OpenExternal(url string) error {
	runtime.BrowserOpenURL(a.ctx, url)
	return nil
}

func (a *App) SelectDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Directory",
	})
}
