//go:build desktop && darwin

// On macOS the app menu, shown in the menu bar while Zonn Node is active
// (click its Dock icon), stands in for a tray icon: it reopens the window,
// pauses or resumes, and quits.
package main

import (
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
)

func (a *App) startTray() {}

func (a *App) appMenu() *menu.Menu {
	m := menu.NewMenu()
	node := m.AddSubmenu("Zonn Node")
	node.AddText("Show Zonn Node", keys.CmdOrCtrl("0"), func(*menu.CallbackData) { a.showWindow() })
	node.AddText("Pause or resume contributing", keys.CmdOrCtrl("p"), func(*menu.CallbackData) { a.togglePause() })
	node.AddSeparator()
	node.AddText("Hide", keys.CmdOrCtrl("h"), func(*menu.CallbackData) { a.hideWindow() })
	node.AddText("Quit Zonn Node", keys.CmdOrCtrl("q"), func(*menu.CallbackData) { a.quit() })
	m.Append(menu.EditMenu())
	return m
}
