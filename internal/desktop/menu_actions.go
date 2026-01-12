package desktop

import (
	"fmt"

	"gestalt/internal/version"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) ShowAbout() {
	if a == nil || a.ctx == nil {
		return
	}
	versionLabel := version.Version
	if versionLabel == "" {
		versionLabel = "dev"
	}
	message := fmt.Sprintf("Gestalt %s\nMulti-terminal AI agent dashboard.", versionLabel)
	if _, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:    runtime.InfoDialog,
		Title:   "About Gestalt",
		Message: message,
	}); err != nil && a.logger != nil {
		a.logger.Warn("about dialog failed", map[string]string{
			"error": err.Error(),
		})
	}
}

func (a *App) EmitMenuEvent(eventName string) {
	if a == nil || a.ctx == nil || eventName == "" {
		return
	}
	runtime.WindowExecJS(a.ctx, fmt.Sprintf("window.dispatchEvent(new CustomEvent(%q))", eventName))
}
