//go:build desktop && !darwin

// The tray icon on Windows and Linux. On Linux it uses the StatusNotifier
// D-Bus protocol (GNOME with the AppIndicator extension, which Ubuntu ships,
// KDE, and most other desktops); where no tray exists the app keeps running
// in the background and launching it again from the app menu reopens the
// window. macOS uses the app menu instead (tray_darwin.go).
package main

import (
	_ "embed"
	"runtime"
	"time"

	"fyne.io/systray"
	"github.com/wailsapp/wails/v2/pkg/menu"
)

//go:embed build/tray.png
var trayPNG []byte

//go:embed build/tray.ico
var trayICO []byte

// appMenu is the window menu bar: none on Windows and Linux.
func (a *App) appMenu() *menu.Menu { return nil }

// startTray runs the tray on its own locked OS thread, which Windows needs
// for the tray's message loop.
func (a *App) startTray() {
	go func() {
		runtime.LockOSThread()
		systray.Run(a.onTrayReady, func() {})
	}()
}

func (a *App) onTrayReady() {
	if runtime.GOOS == "windows" {
		systray.SetIcon(trayICO)
	} else {
		systray.SetIcon(trayPNG)
	}
	systray.SetTitle("Zonn Node")
	systray.SetTooltip("Zonn Node")
	status := systray.AddMenuItem("Starting", "What this machine is doing")
	status.Disable()
	systray.AddSeparator()
	open := systray.AddMenuItem("Open Zonn Node", "Show the window")
	pause := systray.AddMenuItem("Pause contributing", "Stop or resume contributing this machine")
	update := systray.AddMenuItem("Install update", "Download, check, and open the newer Zonn Node")
	update.Hide()
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit Zonn Node", "Stop contributing and close the app")

	refresh := func() {
		st := a.daemon.Snapshot()
		status.SetTitle(statusLine(st))
		systray.SetTooltip("Zonn Node: " + statusLine(st))
		if st.Available {
			pause.SetTitle("Pause contributing")
		} else {
			pause.SetTitle("Resume contributing")
		}
		switch {
		case !st.Update.Available || st.Update.Status == "opened":
			update.Hide()
		case st.Update.Status == "downloading":
			update.SetTitle("Downloading Zonn Node " + st.Update.Version)
			update.Disable()
			update.Show()
		default:
			update.SetTitle("Install Zonn Node " + st.Update.Version)
			update.Enable()
			update.Show()
		}
	}
	refresh()
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-open.ClickedCh:
				a.showWindow()
			case <-pause.ClickedCh:
				a.togglePause()
				refresh()
			case <-update.ClickedCh:
				if err := a.daemon.InstallUpdate(); err == nil {
					a.showWindow() // the window shows the download's progress
				}
				refresh()
			case <-quit.ClickedCh:
				systray.Quit()
				a.quit()
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()
}
