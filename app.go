//go:build desktop

// This file is the Wails application layer. It only builds with the `desktop`
// tag on a machine that has a webview (webkit2gtk on Linux). The engine it wraps
// lives in internal/daemon and runs headless everywhere else, so the window is a
// pure viewer. See README for how to build the desktop binary.
package main

import (
	"context"
	"log"
	"os"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/zonnhq/node-gui/internal/autostart"
	"github.com/zonnhq/node-gui/internal/config"
	"github.com/zonnhq/node-gui/internal/daemon"
	"github.com/zonnhq/node-gui/internal/localapi"
)

// App is the Wails bound application.
type App struct {
	ctx    context.Context
	daemon *daemon.Daemon
}

// NewApp builds the app.
func NewApp() *App { return &App{} }

// startup loads config, starts the daemon and the local API, and begins the
// Coordinator lifecycle. Called by Wails when the window comes up.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	cfg, err := config.Load()
	if err != nil {
		log.Printf("load config: %v", err)
		cfg = config.Default()
	}

	a.daemon = daemon.New(cfg)
	a.daemon.SetDesktop(true)
	// Start at sign in follows the setting, and is rewritten at every start
	// so it always points at the installed executable.
	a.applyAutoStart(cfg)
	a.daemon.OnConfigChange(a.applyAutoStart)

	api := localapi.New(a.daemon, localapi.DefaultAddr)
	go func() {
		if err := api.ListenAndServe(); err != nil {
			log.Printf("local api stopped: %v", err)
		}
	}()

	go a.daemon.Run(ctx)
	a.startTray()
}

// applyAutoStart makes starting at sign in match the setting.
func (a *App) applyAutoStart(cfg config.Config) {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("autostart: %v", err)
		return
	}
	if err := autostart.Set(cfg.AutoStart, exe); err != nil {
		log.Printf("autostart: %v", err)
		a.daemon.PushError("WARN", "Could not change starting at sign in: "+err.Error())
	}
}

// showWindow brings the window back from the background.
func (a *App) showWindow() {
	if a.ctx == nil {
		return
	}
	runtime.WindowUnminimise(a.ctx)
	runtime.Show(a.ctx)
	runtime.WindowShow(a.ctx)
}

// hideWindow sends the window to the background; the node keeps running.
func (a *App) hideWindow() {
	if a.ctx != nil {
		runtime.WindowHide(a.ctx)
	}
}

// togglePause flips the contributor's Available switch, as the Settings
// switch does.
func (a *App) togglePause() {
	cfg := a.daemon.Config()
	cfg.Available = !cfg.Available
	if err := a.daemon.UpdateConfig(cfg); err != nil {
		log.Printf("pause: %v", err)
	}
}

// quit stops the node and the app.
func (a *App) quit() {
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

// statusLine says in a few words what the node is doing, for the tray.
func statusLine(st daemon.State) string {
	switch {
	case !st.Available:
		return "Paused"
	case st.WaitingForIdle:
		return "Waiting until you are away"
	case !st.Enrolled:
		return "Not enrolled"
	case st.Connection == daemon.Connected && st.ColonyUsage.Active:
		return "Computing for a Zone"
	case st.Connection == daemon.Connected:
		return "Contributing, ready for work"
	case st.Connection == daemon.UpdateRequired:
		return "Update required"
	default:
		return "Connecting"
	}
}

// The methods below are bound into the frontend by Wails. The frontend mainly
// reads the local API on :9090, but these give it a direct path too.

// State returns the current daemon snapshot.
// shutdown stops the node's stage workers when the app quits.
func (a *App) shutdown(ctx context.Context) {
	if a.daemon != nil {
		a.daemon.Shutdown()
	}
}

func (a *App) State() daemon.State { return a.daemon.Snapshot() }

// GetConfig returns the current configuration.
func (a *App) GetConfig() config.Config { return a.daemon.Config() }

// SaveConfig applies and persists new settings.
func (a *App) SaveConfig(c config.Config) error { return a.daemon.UpdateConfig(c) }
