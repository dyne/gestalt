package desktop

import (
	"fmt"

	"gestalt/internal/version"
)

func (a *App) ShowAbout() {
	if a == nil || a.app == nil {
		return
	}
	versionLabel := version.Version
	if versionLabel == "" {
		versionLabel = "dev"
	}
	message := fmt.Sprintf("Gestalt %s\nMulti-terminal AI agent dashboard.", versionLabel)
	dialog := a.app.Dialog.Info().
		SetTitle("About Gestalt").
		SetMessage(message)
	if a.window != nil {
		dialog.AttachToWindow(a.window)
	}
	dialog.Show()
}

func (a *App) EmitMenuEvent(eventName string) {
	if a == nil || a.window == nil || eventName == "" {
		return
	}
	a.window.ExecJS(fmt.Sprintf("window.dispatchEvent(new CustomEvent(%q))", eventName))
}
