// Package config reads and writes the contributor's node settings at
// ~/.zonn/config.json. It is the single place the user's allocation choices
// and the Coordinator address are stored. Versions before the rename used
// ~/.colony, which is moved to ~/.zonn on first start (see migrate.go).
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Allocation is how much of the machine the user donates.
type Allocation struct {
	CPUCores      int `json:"cpu_cores"`
	RAMGB         int `json:"ram_gb"`
	GPUMemory     int `json:"gpu_memory_gb"`
	BandwidthMbps int `json:"bandwidth_mbps"`
}

// Config is the persisted node configuration.
type Config struct {
	NodeID         string     `json:"node_id"`
	NodeName       string     `json:"node_name"`
	CoordinatorURL string     `json:"coordinator_url"`
	Allocation     Allocation `json:"allocation"`
	// Available is the operator's on/off switch: when false the node stops
	// contributing and drops offline from the Zone until it is turned back on.
	Available bool `json:"available"`
	// Configured records that the starting allocation has been seeded. On the
	// very first run it is false, so the daemon fills in a sensible 20% default
	// instead of leaving everything at zero; after that the user's choices stand.
	Configured bool `json:"configured"`
	// OnlyWhenIdle contributes only while the owner has been away (no keyboard
	// or mouse input) for at least IdleMinutes, and stops as soon as they
	// come back.
	OnlyWhenIdle bool `json:"only_when_idle"`
	IdleMinutes  int  `json:"idle_minutes"`
	// AutoStart starts Zonn Node, hidden, when the user signs in. Only the
	// desktop app acts on it.
	AutoStart bool `json:"auto_start"`
	// CrashReportsEnabled sends an anonymized report (error type, stack trace,
	// operating system only, never hostname, username, or IP) to the Coordinator
	// if the daemon recovers from a panic, so the operator can see it happened.
	// Defaults on for the beta; the contributor can turn it off in Settings.
	CrashReportsEnabled bool `json:"crash_reports_enabled"`
	// ShareCommunity lets this machine join community Zones, run by Zonn for
	// other people, as well as its owner's own Zones. Off unless the
	// contributor turns it on.
	ShareCommunity bool `json:"share_community"`
	// ModelCacheGB caps how much disk the downloaded model weights may use;
	// past it the least recently used models not in use are removed. Zero
	// means no limit.
	ModelCacheGB int `json:"model_cache_gb"`
}

var writeMu sync.Mutex

// DefaultIdleMinutes is how long the owner must be away before a node with
// Only when idle contributes; MaxIdleMinutes caps the setting.
const (
	DefaultIdleMinutes = 5
	MaxIdleMinutes     = 120
)

// DefaultModelCacheGB is the model cache cap of a new install, and of a
// config from before the setting existed.
const DefaultModelCacheGB = 20

// Dir returns the node's config directory, creating it if needed. It is
// ~/.zonn (moved there from ~/.colony on first start after an upgrade), but
// ZONN_HOME overrides it so several nodes can run on one machine without
// changing HOME (which would hide the user's Python packages).
func Dir() (string, error) {
	dir := Env("ZONN_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = defaultDir(home)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Env reads a ZONN_ environment variable, falling back to its COLONY_ name
// from before the rename, so existing setups keep working.
func Env(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	if rest, ok := strings.CutPrefix(name, "ZONN_"); ok {
		return os.Getenv("COLONY_" + rest)
	}
	return ""
}

// Path returns the config file path.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Default builds a fresh config with a stable id and a friendly name.
func Default() Config {
	return Config{
		NodeID:              NewID(),
		NodeName:            fmt.Sprintf("PC-%s", randomSuffix(6)),
		CoordinatorURL:      "localhost:8080",
		Allocation:          Allocation{},
		Available:           true,
		Configured:          false,
		OnlyWhenIdle:        false,
		IdleMinutes:         DefaultIdleMinutes,
		AutoStart:           false,
		CrashReportsEnabled: true,
		ModelCacheGB:        DefaultModelCacheGB,
	}
}

// NewID returns a random stable node id.
func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand should not fail, but fall back to a non-fatal value.
		return "node-" + randomSuffix(16)
	}
	return "node-" + hex.EncodeToString(b)
}

// Load reads the config file, or returns a saved default if none exists yet.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := Default()
		if err := Save(cfg); err != nil {
			return cfg, err
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	// A config written before a field existed omits it, so a missing bool would
	// read as false. Detect the absent keys and apply the intended defaults so an
	// upgrade does not silently pause an existing node or rename it.
	var present map[string]json.RawMessage
	_ = json.Unmarshal(data, &present)
	rewrite := false
	if _, ok := present["available"]; !ok {
		cfg.Available = true
		rewrite = true
	}
	// An existing config file has already been through the user's hands, so mark
	// it configured to protect its allocation from the first run 20% seeding.
	if _, ok := present["configured"]; !ok {
		cfg.Configured = true
		rewrite = true
	}
	// A config written before this field existed should still get crash reports
	// by default, matching a fresh install rather than silently opting out.
	if _, ok := present["crash_reports_enabled"]; !ok {
		cfg.CrashReportsEnabled = true
		rewrite = true
	}
	if cfg.IdleMinutes <= 0 {
		cfg.IdleMinutes = DefaultIdleMinutes
		rewrite = true
	}
	if _, ok := present["model_cache_gb"]; !ok {
		cfg.ModelCacheGB = DefaultModelCacheGB
		rewrite = true
	}
	// Backfill a stable id for configs written before it existed.
	if cfg.NodeID == "" {
		cfg.NodeID = NewID()
		rewrite = true
	}
	if rewrite {
		if err := Save(cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

// Save writes the config atomically.
func Save(cfg Config) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	path, err := Path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func randomSuffix(n int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[mrand.Intn(len(alphabet))]
	}
	return string(b)
}
