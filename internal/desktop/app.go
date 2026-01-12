package desktop

import (
	"context"
	"errors"
	"net/http"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/terminal"
	"gestalt/internal/version"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type App struct {
	ctx          context.Context
	serverURL    string
	manager      *terminal.Manager
	server       *http.Server
	logger       *logging.Logger
	app          *application.App
	window       *application.WebviewWindow
}

func NewApp(url string, manager *terminal.Manager, server *http.Server, logger *logging.Logger) *App {
	return &App{
		serverURL: url,
		manager:   manager,
		server:    server,
		logger:    logger,
	}
}

func (a *App) AttachRuntime(app *application.App, window *application.WebviewWindow) {
	if a == nil {
		return
	}
	a.app = app
	a.window = window
}

func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.ctx = ctx
	return nil
}

func (a *App) ServiceShutdown() error {
	a.Shutdown()
	return nil
}

func (a *App) Shutdown() {
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
}

func (a *App) GetServerURL() string {
	return a.serverURL
}

func (a *App) GetVersion() string {
	return version.Version
}

func (a *App) OpenExternal(url string) error {
	if a == nil || a.app == nil {
		return errors.New("desktop application not initialized")
	}
	return a.app.Browser.OpenURL(url)
}

func (a *App) SelectDirectory() (string, error) {
	if a == nil || a.app == nil {
		return "", errors.New("desktop application not initialized")
	}
	dialog := a.app.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:                "Select Directory",
		CanChooseDirectories: true,
		CanChooseFiles:       false,
	})
	if a.window != nil {
		dialog.AttachToWindow(a.window)
	}
	return dialog.PromptForSingleSelection()
}

func (a *App) Quit() {
	if a == nil || a.app == nil {
		return
	}
	a.app.Quit()
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
