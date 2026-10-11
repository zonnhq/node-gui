//go:build desktop

// This file is the Wails application layer. It only builds with the `desktop`
// tag on a machine that has a webview (webkit2gtk on Linux). The engine it wraps
// lives in internal/daemon and runs headless everywhere else, so the window is a
// pure viewer. See README for how to build the desktop binary.
package main

import (
	"context"
	"log"

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

	api := localapi.New(a.daemon, localapi.DefaultAddr)
	go func() {
		if err := api.ListenAndServe(); err != nil {
			log.Printf("local api stopped: %v", err)
		}
	}()

	go a.daemon.Run(ctx)
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
