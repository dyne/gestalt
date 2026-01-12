package desktop

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

const documentationURL = "https://dyne.org/gestalt"

func BuildMenu(app *App) *application.Menu {
	menuBar := application.NewMenu()

	fileMenu := menuBar.AddSubmenu("File")
	fileMenu.Add("New Terminal").
		SetAccelerator("CmdOrCtrl+N").
		OnClick(func(_ *application.Context) {
			if app == nil {
				return
			}
			app.EmitMenuEvent("gestalt:menu:new-terminal")
		})
	fileMenu.AddSeparator()
	fileMenu.Add("Quit").
		SetAccelerator("CmdOrCtrl+Q").
		OnClick(func(_ *application.Context) {
			if app == nil {
				return
			}
			app.Quit()
		})

	menuBar.AddRole(application.EditMenu)

	viewMenu := menuBar.AddSubmenu("View")
	viewMenu.Add("Toggle Developer Tools").
		SetAccelerator("CmdOrCtrl+OptionOrAlt+I").
		OnClick(func(_ *application.Context) {
			if app == nil {
				return
			}
			app.EmitMenuEvent("gestalt:menu:toggle-devtools")
		})

	helpMenu := menuBar.AddSubmenu("Help")
	helpMenu.Add("Documentation").OnClick(func(_ *application.Context) {
		if app == nil {
			return
		}
		_ = app.OpenExternal(documentationURL)
	})
	helpMenu.Add("About Gestalt").OnClick(func(_ *application.Context) {
		if app == nil {
			return
		}
		app.ShowAbout()
	})

	return menuBar
}
