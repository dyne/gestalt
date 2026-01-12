package desktop

import (
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
)

const documentationURL = "https://dyne.org/gestalt"

func BuildMenu(app *App) *menu.Menu {
	menuBar := menu.NewMenu()

	fileMenu := menuBar.AddSubmenu("File")
	fileMenu.AddText("New Terminal", keys.CmdOrCtrl("n"), func(_ *menu.CallbackData) {
		if app == nil {
			return
		}
		app.EmitMenuEvent("gestalt:menu:new-terminal")
	})
	fileMenu.AddSeparator()
	fileMenu.AddText("Quit", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) {
		if app == nil {
			return
		}
		app.Quit()
	})

	menuBar.Append(menu.EditMenu())

	viewMenu := menuBar.AddSubmenu("View")
	viewMenu.AddText("Toggle Developer Tools", keys.Combo("i", keys.CmdOrCtrlKey, keys.OptionOrAltKey), func(_ *menu.CallbackData) {
		if app == nil {
			return
		}
		app.EmitMenuEvent("gestalt:menu:toggle-devtools")
	})

	helpMenu := menuBar.AddSubmenu("Help")
	helpMenu.AddText("Documentation", nil, func(_ *menu.CallbackData) {
		if app == nil {
			return
		}
		_ = app.OpenExternal(documentationURL)
	})
	helpMenu.AddText("About Gestalt", nil, func(_ *menu.CallbackData) {
		if app == nil {
			return
		}
		app.ShowAbout()
	})

	return menuBar
}
