package desktop

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/terminal"
	"gestalt/internal/version"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx          context.Context
	serverURL    string
	manager      *terminal.Manager
	server       *http.Server
	shutdown     chan struct{}
	shutdownOnce sync.Once
	logger       *logging.Logger
}

func NewApp(url string, manager *terminal.Manager, server *http.Server, logger *logging.Logger) *App {
	return &App{
		serverURL: url,
		manager:   manager,
		server:    server,
		shutdown:  make(chan struct{}),
		logger:    logger,
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	runtime.WindowCenter(ctx)
}

func (a *App) Shutdown(ctx context.Context) {
	if a.server != nil {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.server.Shutdown(shutdownContext); err != nil && a.logger != nil {
			a.logger.Warn("desktop server shutdown failed", map[string]string{
				"error": err.Error(),
			})
		}
	}
	a.shutdownSessions()
	if a.shutdown != nil {
		a.shutdownOnce.Do(func() {
			close(a.shutdown)
		})
	}
}

func (a *App) BeforeClose(ctx context.Context) bool {
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

func (a *App) Quit() {
	if a.ctx == nil {
		return
	}
	runtime.Quit(a.ctx)
}

func (a *App) ShutdownDone() <-chan struct{} {
	if a == nil {
		return nil
	}
	return a.shutdown
}

func (a *App) shutdownSessions() {
	if a == nil || a.manager == nil {
		return
	}
	for _, session := range a.manager.List() {
		if session.ID == "" {
			continue
		}
		if err := a.manager.Delete(session.ID); err != nil && !errors.Is(err, terminal.ErrSessionNotFound) {
			if a.logger != nil {
				a.logger.Warn("desktop session shutdown failed", map[string]string{
					"terminal_id": session.ID,
					"error":       err.Error(),
				})
			}
		}
	}
}
