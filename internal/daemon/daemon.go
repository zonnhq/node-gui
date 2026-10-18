// Package daemon is the node's engine. It detects hardware, talks to the
// Coordinator, and keeps a live snapshot of state for the local API and the GUI.
// It is designed to run headless: the Wails window is just a viewer on top.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zonnhq/node-gui/internal/client"
	"github.com/zonnhq/node-gui/internal/config"
	"github.com/zonnhq/node-gui/internal/idle"
	"github.com/zonnhq/node-gui/internal/modelcache"
	"github.com/zonnhq/node-gui/internal/resources"
	"github.com/zonnhq/node-gui/internal/version"
	"github.com/zonnhq/node-gui/internal/worker"
)

// Connection states between the node and the Coordinator.
const (
	Connecting   = "CONNECTING"
	Connected    = "CONNECTED"
	Disconnected = "DISCONNECTED"
	// Paused means the operator turned availability off, so the node is not
	// contributing and is not trying to reach the Coordinator.
	Paused = "PAUSED"
	// Unenrolled means the node has no credential for the configured
	// Coordinator and waits for an enrollment code from Zonn Console.
	Unenrolled = "UNENROLLED"
	// UpdateRequired means the Coordinator refuses this Zonn Node version.
	UpdateRequired = "UPDATE_REQUIRED"
)

const (
	minBackoff  = 1 * time.Second
	maxBackoff  = 30 * time.Second
	maxEvents   = 200
	registerTTL = 8 * time.Second
)

// Event is one line in the activity log.
type Event struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// Ranking is the node's live standing in the fleet.
type Ranking struct {
	Rank         int     `json:"rank"`
	ActiveNodes  int     `json:"active_nodes"`
	Score        float64 `json:"contribution_score"`
	AverageScore float64 `json:"average_score"`
}

// Community is this machine's standing for community Zones, as the
// Coordinator reports it on every heartbeat.
type Community struct {
	Status   string `json:"status"` // one sentence; empty from older Coordinators
	Eligible bool   `json:"eligible"`
}

// ColonyUsage is how much of the contribution the Zone is actually using right
// now, measured from the worker process. It is zero when no job is running.
type ColonyUsage struct {
	CPUCores float64 `json:"cpu_cores"`
	RAMGB    float64 `json:"ram_gb"`
	Active   bool    `json:"active"`
}

// State is an immutable snapshot handed to the API and the GUI.
type State struct {
	NodeName       string                `json:"node_name"`
	NodeID         string                `json:"node_id"`
	Connection     string                `json:"connection"`
	Status         string                `json:"status"`
	Available      bool                  `json:"available"`
	ColonyID       string                `json:"colony_id"`
	CoordinatorURL string                `json:"coordinator_url"`
	Specs          resources.Specs       `json:"specs"`
	Allocation     config.Allocation     `json:"allocation"`
	Utilization    resources.Utilization `json:"utilization"`
	ColonyUsage    ColonyUsage           `json:"colony_usage"`
	Ranking        Ranking               `json:"ranking"`
	Community      Community             `json:"community"`
	Events         []Event               `json:"events"`
	// Enrolled is true when the node holds a credential for the configured
	// Coordinator.
	Enrolled bool `json:"enrolled"`
	// Notice explains a state the contributor has to act on, such as an
	// update being required.
	Notice     string `json:"notice"`
	AppVersion string `json:"app_version"`
	// WaitingForIdle is true while Only when idle holds the node back, with
	// IdleNote saying why.
	WaitingForIdle bool   `json:"waiting_for_idle"`
	IdleNote       string `json:"idle_note"`
	// Desktop is true when the desktop app runs the node, so settings only it
	// acts on (start on sign in) can be offered.
	Desktop bool `json:"desktop"`
	// Update is a newer Zonn Node, when one is published.
	Update UpdateState `json:"update"`
	// Downloads are the model weights stage workers are downloading now.
	Downloads []worker.Download `json:"downloads"`
	// ModelCache is what the downloaded models take on disk.
	ModelCache CacheState `json:"model_cache"`
}

// CacheState describes the model cache: its folder, cap, and models.
type CacheState struct {
	Dir    string             `json:"dir"`
	CapGB  int                `json:"cap_gb"` // zero means no limit
	Bytes  int64              `json:"bytes"`
	Models []modelcache.Model `json:"models"`
}

// Daemon holds the mutable state behind a mutex.
type Daemon struct {
	mu          sync.RWMutex
	cfg         config.Config
	specs       resources.Specs
	fingerprint string
	nodeID      string
	connection  string
	colonyID    string
	util        resources.Utilization
	ranking     Ranking
	community   Community
	events      []Event
	conn        *client.Client
	gpuOverTemp bool
	// workers are this machine's running stage workers, by deployment and
	// role, with their live resource draw.
	workers map[string]*runningWorker
	creds   *config.Credentials
	notice  string
	// accelerator is what the inference runtime can drive (cuda or mps),
	// learned once by probing it in the background; empty means CPU only.
	accelerator string
	// features is what else the runtime supports (int8 and int4 quantized
	// stages), from the same probe.
	features []string

	// idleSource reads the owner's idle time; idleBlocked is set while Only
	// when idle holds the node back, with idleNote saying why.
	idleSource  idle.Source
	idleBlocked bool
	idleNote    string
	desktop     bool
	onConfig    []func(config.Config)
	update      UpdateState
	// cache is the model cache (nil when its folder cannot be made), and
	// cacheState its last scan, refreshed whenever it may have changed.
	cache      *modelcache.Cache
	cacheState CacheState

	reconnect chan struct{}
}

// idleCheckInterval is how often Only when idle looks at the idle time. The
// owner coming back is noticed within this long.
const idleCheckInterval = 5 * time.Second

// SetDesktop marks the daemon as run by the desktop app.
func (d *Daemon) SetDesktop(on bool) {
	d.mu.Lock()
	d.desktop = on
	d.mu.Unlock()
}

// SetIdleSource replaces how the owner's idle time is read (for tests).
func (d *Daemon) SetIdleSource(s idle.Source) {
	d.mu.Lock()
	d.idleSource = s
	d.mu.Unlock()
}

// OnConfigChange registers a callback run after every saved settings change.
func (d *Daemon) OnConfigChange(fn func(config.Config)) {
	d.mu.Lock()
	d.onConfig = append(d.onConfig, fn)
	d.mu.Unlock()
}

// checkIdle applies Only when idle: it decides from the owner's idle time
// whether the node may contribute and, when that changes, wakes the run loop
// so the node pauses or resumes. It returns the decision.
func (d *Daemon) checkIdle() idle.Decision {
	d.mu.RLock()
	only, minutes, src := d.cfg.OnlyWhenIdle, d.cfg.IdleMinutes, d.idleSource
	d.mu.RUnlock()
	var away time.Duration
	var err error
	if only {
		away, err = src.Idle()
	}
	decision := idle.Decide(only, time.Duration(minutes)*time.Minute, away, err)
	d.mu.Lock()
	changed := d.idleBlocked == decision.Allowed
	d.idleBlocked = !decision.Allowed
	d.idleNote = decision.Note
	d.mu.Unlock()
	if changed {
		if decision.Allowed {
			d.log("INFO", "You have been away long enough; contributing again (Only when idle)")
		} else {
			d.log("INFO", decision.Note)
		}
		d.triggerReconnect()
	}
	return decision
}

// watchIdle runs checkIdle on a clock until ctx ends.
func (d *Daemon) watchIdle(ctx context.Context) {
	defer d.recoverCrash("watchIdle")
	ticker := time.NewTicker(idleCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.checkIdle()
		}
	}
}

// runningWorker is one stage worker this node is running for a Zone.
type runningWorker struct {
	cancel   context.CancelFunc
	usage    worker.Usage
	model    string           // the model it serves, kept in the cache while it runs
	download *worker.Download // its download in progress, if any
}

// gpuWarnTempC is the GPU temperature above which the node warns the operator.
const gpuWarnTempC = 90.0

// New detects hardware and builds a daemon from the given config. On the first
// run it seeds a starting contribution of 20% of the machine so a new node
// arrives with something to give rather than zeros the user must dial up.
func New(cfg config.Config) *Daemon {
	specs := resources.Detect()
	seeded := false
	if !cfg.Configured {
		cfg.Allocation = defaultAllocation(specs)
		cfg.Configured = true
		seeded = true
	}
	cfg.Allocation = clampAllocation(cfg.Allocation, specs)
	d := &Daemon{
		cfg:         cfg,
		specs:       specs,
		fingerprint: resources.Fingerprint(),
		connection:  Disconnected,
		reconnect:   make(chan struct{}, 1),
		workers:     make(map[string]*runningWorker),
		idleSource:  idle.System(),
	}
	if creds, ok := config.LoadCredentials(); ok {
		d.creds = &creds
	}
	if dir, err := modelcache.Dir(); err == nil {
		d.cache = modelcache.New(dir)
	} else {
		d.log("WARN", "Could not make the model cache folder: "+err.Error())
	}
	d.log("INFO", "Detected "+specs.OS+" with "+humanSpecs(specs))
	if seeded {
		if err := config.Save(cfg); err != nil {
			d.log("WARN", "Could not save the starting allocation: "+err.Error())
		} else {
			d.log("INFO", "Seeded a starting contribution of 20% of this machine; adjust it in Settings")
		}
	}
	return d
}

// defaultAllocation is the first run contribution: 20% of each detected resource,
// and 200 Mbps (20% of the 1000 Mbps bandwidth range).
func defaultAllocation(s resources.Specs) config.Allocation {
	pct := func(total int) int { return int(math.Round(float64(total) * 0.2)) }
	return config.Allocation{
		CPUCores:      pct(s.CPUCores),
		RAMGB:         pct(s.RAMGB),
		GPUMemory:     pct(s.GPUMemoryGB),
		BandwidthMbps: 200,
	}
}

// Specs returns the detected hardware.
func (d *Daemon) Specs() resources.Specs {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.specs
}

// Config returns the current config.
func (d *Daemon) Config() config.Config {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg
}

// Snapshot returns the current state, events newest first.
func (d *Daemon) Snapshot() State {
	d.mu.RLock()
	defer d.mu.RUnlock()

	events := make([]Event, len(d.events))
	for i, e := range d.events {
		events[len(d.events)-1-i] = e
	}
	// Stages of one model share its download in the cache, so it is shown
	// once, at the furthest any of them has seen.
	downloads := []worker.Download{}
	seen := map[string]int{}
	for _, w := range d.workers {
		if w.download == nil {
			continue
		}
		if i, ok := seen[w.download.Model]; ok {
			if w.download.Done > downloads[i].Done {
				downloads[i] = *w.download
			}
			continue
		}
		seen[w.download.Model] = len(downloads)
		downloads = append(downloads, *w.download)
	}
	cacheState := d.cacheState
	cacheState.CapGB = d.cfg.ModelCacheGB
	cacheState.Models = append([]modelcache.Model{}, d.cacheState.Models...)
	status := "OFFLINE"
	if d.connection == Connected {
		status = "ONLINE"
	}
	return State{
		NodeName:       d.cfg.NodeName,
		NodeID:         d.nodeID,
		Connection:     d.connection,
		Status:         status,
		Available:      d.cfg.Available,
		ColonyID:       d.colonyID,
		CoordinatorURL: d.cfg.CoordinatorURL,
		Specs:          d.specs,
		Allocation:     d.cfg.Allocation,
		Utilization:    d.util,
		ColonyUsage: ColonyUsage{
			CPUCores: d.zoneUsageLocked().CPUCores,
			RAMGB:    d.zoneUsageLocked().RAMGB,
			Active:   len(d.workers) > 0,
		},
		Ranking:        d.ranking,
		Community:      d.community,
		Events:         events,
		Enrolled:       d.enrolledLocked(),
		Notice:         d.notice,
		AppVersion:     version.Version,
		WaitingForIdle: d.cfg.Available && d.idleBlocked,
		IdleNote:       d.idleNote,
		Desktop:        d.desktop,
		Update:         d.update,
		Downloads:      downloads,
		ModelCache:     cacheState,
	}
}

// enrolledLocked reports whether the node holds a credential for the
// configured Coordinator. Callers hold d.mu.
func (d *Daemon) enrolledLocked() bool {
	return d.creds != nil && d.creds.CoordinatorURL == d.cfg.CoordinatorURL
}

// credentials returns a copy of the credential for the configured
// Coordinator, or nil.
func (d *Daemon) credentials() *config.Credentials {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if !d.enrolledLocked() {
		return nil
	}
	c := *d.creds
	return &c
}

// Enroll exchanges a one time code from Zonn Console for this node's
// credential, then connects. The machine becomes the code creator's.
func (d *Daemon) Enroll(ctx context.Context, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return fmt.Errorf("paste the enrollment code from Zonn Console")
	}
	cfg := d.Config()
	cli, err := client.Dial(cfg.CoordinatorURL, nil)
	if err != nil {
		return fmt.Errorf("could not reach the Coordinator at %s: %w", cfg.CoordinatorURL, err)
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(ctx, registerTTL)
	defer cancel()
	creds, err := cli.Enroll(ctx, code, cfg, d.Specs(), d.fingerprint)
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() != codes.Unavailable {
			return fmt.Errorf("%s", st.Message())
		}
		return fmt.Errorf("could not reach the Coordinator at %s: %s", cfg.CoordinatorURL, status.Convert(err).Message())
	}
	if err := config.SaveCredentials(creds); err != nil {
		return fmt.Errorf("enrolled, but could not save the credential: %w", err)
	}
	d.mu.Lock()
	d.creds = &creds
	d.notice = ""
	d.cfg.NodeID = creds.NodeID
	newCfg := d.cfg
	d.mu.Unlock()
	if err := config.Save(newCfg); err != nil {
		d.log("WARN", "Could not save the node id: "+err.Error())
	}
	d.log("INFO", "Enrolled with the Coordinator as node "+creds.NodeID)
	d.triggerReconnect()
	return nil
}

// UpdateConfig validates and saves new settings, then forces a reconnect so the
// Coordinator sees the updated allocation right away.
func (d *Daemon) UpdateConfig(newCfg config.Config) error {
	d.mu.Lock()
	newCfg.Allocation = clampAllocation(newCfg.Allocation, d.specs)
	if newCfg.ModelCacheGB < 0 {
		newCfg.ModelCacheGB = 0
	}
	capChanged := newCfg.ModelCacheGB != d.cfg.ModelCacheGB
	if newCfg.NodeName == "" {
		newCfg.NodeName = d.cfg.NodeName
	}
	if newCfg.CoordinatorURL == "" {
		newCfg.CoordinatorURL = d.cfg.CoordinatorURL
	}
	if newCfg.IdleMinutes <= 0 {
		newCfg.IdleMinutes = config.DefaultIdleMinutes
	}
	if newCfg.IdleMinutes > config.MaxIdleMinutes {
		newCfg.IdleMinutes = config.MaxIdleMinutes
	}
	d.cfg = newCfg
	hooks := append([]func(config.Config){}, d.onConfig...)
	d.mu.Unlock()

	if err := config.Save(newCfg); err != nil {
		return err
	}
	d.log("INFO", "Settings updated, applying the changes")
	d.checkIdle()
	if capChanged {
		d.fitCache()
	}
	for _, fn := range hooks {
		fn(newCfg)
	}
	d.triggerReconnect()
	return nil
}

// available reports whether the node should contribute now: its owner has it
// switched on and, with Only when idle, has been away long enough.
func (d *Daemon) available() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg.Available && !d.idleBlocked
}

// Run drives the connect, register, and heartbeat lifecycle until ctx is done.
// It never returns on transient errors, it backs off and retries. When the
// operator turns availability off, it parks in the Paused state and waits to be
// woken by a config change rather than hammering the Coordinator.
func (d *Daemon) Run(ctx context.Context) {
	defer d.recoverCrash("Run")
	go d.probeAccelerator(ctx)
	d.checkIdle()
	go d.watchIdle(ctx)
	go d.watchUpdates(ctx)
	go d.watchCache(ctx)
	backoff := minBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		if !d.available() {
			d.park(ctx)
			backoff = minBackoff
			continue
		}
		if d.credentials() == nil {
			d.waitForEnrollment(ctx)
			backoff = minBackoff
			continue
		}
		if d.connectOnce(ctx) {
			backoff = minBackoff // a clean session resets the backoff
		}
		if st := d.Snapshot(); st.Connection == UpdateRequired || (st.Connection == Disconnected && st.Notice != "") {
			// Nothing changes until the app is updated or the Zone lowers its
			// minimum version, so check back slowly.
			backoff = maxBackoff
		}
		if ctx.Err() != nil {
			return
		}
		if !d.available() {
			continue // turned off during the session, park without waiting out the backoff
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		case <-d.reconnect:
		}
		backoff = nextBackoff(backoff)
	}
}

// probeAccelerator learns, once, whether the inference runtime can drive a
// GPU, and re-registers so the Coordinator can plan GPU stages on this
// machine. A GPU the runtime cannot use (for example with a CPU only build of
// the runtime) is never reported.
func (d *Daemon) probeAccelerator(ctx context.Context) {
	defer d.recoverCrash("probeAccelerator")
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	acc, err := worker.Probe(probeCtx)
	if err != nil {
		d.log("WARN", "Could not check the inference runtime for a usable GPU: "+err.Error())
		return
	}
	if len(acc.Features) > 0 {
		d.mu.Lock()
		d.features = acc.Features
		d.mu.Unlock()
	}
	if acc.Device != "cuda" && acc.Device != "mps" {
		if d.Specs().GPUModel != "" {
			d.log("INFO", "The GPU is not usable by this build of the inference runtime, so Zones use the CPU")
		}
		if len(acc.Features) > 0 {
			d.triggerReconnect() // register again with what the runtime supports
		}
		return
	}
	d.mu.Lock()
	d.accelerator = acc.Device
	d.mu.Unlock()
	d.log("INFO", "The inference runtime can use "+acc.Name+" ("+acc.Device+"); Zones can use it when you pledge GPU memory")
	d.triggerReconnect()
}

// waitForEnrollment holds the node in the Unenrolled state until an
// enrollment succeeds or the settings change (a config change or Enroll
// signals d.reconnect).
func (d *Daemon) waitForEnrollment(ctx context.Context) {
	d.setConnection(Unenrolled)
	d.log("INFO", "Waiting to be enrolled: create an enrollment code in Zonn Console and paste it here")
	select {
	case <-ctx.Done():
	case <-d.reconnect:
	}
}

// notifyOffline tells the Coordinator this node is pausing on purpose so the
// fleet reflects it at once instead of waiting out the reaper. Best effort: if
// the Coordinator is unreachable the reaper still catches the silence.
func (d *Daemon) notifyOffline() {
	d.mu.RLock()
	nodeID := d.nodeID
	addr := d.cfg.CoordinatorURL
	d.mu.RUnlock()
	creds := d.credentials()
	if nodeID == "" || creds == nil {
		return
	}
	cli, err := client.Dial(addr, creds)
	if err != nil {
		return
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), registerTTL)
	defer cancel()
	reason := "paused by operator"
	d.mu.RLock()
	if d.cfg.Available && d.idleBlocked {
		reason = "its owner is using it"
	}
	d.mu.RUnlock()
	if err := cli.SetOffline(ctx, nodeID, reason); err != nil {
		d.log("WARN", "Could not tell the Coordinator about the pause: "+err.Error())
	}
}

// park holds the node in the Paused state until availability is turned back on
// (a config change signals d.reconnect) or the context is cancelled. It tells the
// Coordinator up front so the pause shows immediately.
func (d *Daemon) park(ctx context.Context) {
	d.stopWorkers()
	d.notifyOffline()
	d.setConnection(Paused)
	if d.Config().Available {
		d.log("INFO", "Paused while you use this machine (Only when idle)")
	} else {
		d.log("INFO", "Paused: this machine is not available to the Zone")
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.reconnect:
			if d.available() {
				d.log("INFO", "Resumed: making this machine available to the Zone")
				return
			}
			// A config change that left the node unavailable (for example an
			// allocation edit while paused); stay parked.
		}
	}
}

// connectOnce dials, registers, and runs the heartbeat loop for one session.
// It returns true if the session reached the connected state.
func (d *Daemon) connectOnce(ctx context.Context) bool {
	cfg := d.Config()
	d.setConnection(Connecting)
	d.log("INFO", "Connecting to Coordinator at "+cfg.CoordinatorURL)

	creds := d.credentials()
	if creds == nil {
		return false
	}
	cli, err := client.Dial(cfg.CoordinatorURL, creds)
	if err != nil {
		d.log("ERROR", "Could not reach the Coordinator: "+err.Error())
		d.setConnection(Disconnected)
		return false
	}
	defer func() {
		d.mu.Lock()
		d.conn = nil
		d.mu.Unlock()
		cli.Close()
	}()

	d.mu.Lock()
	d.conn = cli
	d.mu.Unlock()

	regCtx, cancel := context.WithTimeout(ctx, registerTTL)
	d.mu.RLock()
	accelerator, features := d.accelerator, d.features
	d.mu.RUnlock()
	res, err := cli.Register(regCtx, creds.NodeID, cfg, d.Specs(), d.fingerprint, accelerator, features)
	cancel()
	if err != nil {
		switch status.Code(err) {
		case codes.Unauthenticated:
			// The Coordinator no longer knows this credential, for example
			// because the machine was enrolled again elsewhere or its
			// database was reset. Only a new enrollment can fix that.
			d.log("ERROR", "The Coordinator no longer accepts this node's credential; enroll it again with a new code from Zonn Console")
			if err := config.ClearCredentials(); err != nil {
				d.log("WARN", "Could not remove the old credential: "+err.Error())
			}
			d.mu.Lock()
			d.creds = nil
			d.mu.Unlock()
			d.setConnection(Unenrolled)
		case codes.PermissionDenied:
			// The owner's account is suspended. Nothing the node does fixes
			// it, so it says so and checks back slowly.
			msg := status.Convert(err).Message()
			d.log("ERROR", msg)
			d.mu.Lock()
			d.notice = msg
			d.mu.Unlock()
			d.setConnection(Disconnected)
		case codes.FailedPrecondition:
			msg := status.Convert(err).Message()
			d.log("ERROR", msg)
			d.mu.Lock()
			d.notice = msg
			d.mu.Unlock()
			d.setConnection(UpdateRequired)
			// Look for the update now, so the window can offer it.
			go d.CheckUpdate(ctx)
		default:
			d.log("ERROR", "Registration failed: "+err.Error())
			d.setConnection(Disconnected)
		}
		return false
	}

	d.mu.Lock()
	d.nodeID = res.NodeID
	d.connection = Connected
	d.notice = ""
	d.mu.Unlock()
	d.log("INFO", "Registered with the Zone, node id "+res.NodeID)

	d.heartbeatLoop(ctx, cli, res.NodeID, res.HeartbeatInterval)

	d.setConnection(Disconnected)
	return true
}

func (d *Daemon) heartbeatLoop(ctx context.Context, cli *client.Client, nodeID string, interval time.Duration) {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := cli.HeartbeatStream(connCtx)
	if err != nil {
		d.log("ERROR", "Could not open the heartbeat stream: "+err.Error())
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.reconnect:
			d.log("INFO", "Restarting the Coordinator connection")
			return
		case <-ticker.C:
			util := resources.Sample(d.specs.CPUCores)
			d.mu.RLock()
			workerUsage := d.zoneUsageLocked()
			workerActive := len(d.workers) > 0
			d.mu.RUnlock()
			ack, err := client.SendHeartbeat(stream, nodeID, util, workerUsage.CPUCores, workerUsage.RAMGB, workerActive)
			if err != nil {
				d.log("WARN", "Lost the Coordinator connection: "+err.Error())
				return
			}
			d.mu.Lock()
			d.util = util
			d.colonyID = ack.GetAssignedColonyId()
			d.ranking = Ranking{
				Rank:         int(ack.GetRank()),
				ActiveNodes:  int(ack.GetActiveNodes()),
				Score:        ack.GetContributionScore(),
				AverageScore: ack.GetAverageScore(),
			}
			d.community = Community{Status: ack.GetCommunityStatus(), Eligible: ack.GetCommunityEligible()}
			d.mu.Unlock()
			d.checkGPUTemp(util.GPUTempC)
			d.handleCommand(ctx, ack.GetCommand())
		}
	}
}

func (d *Daemon) setConnection(state string) {
	d.mu.Lock()
	d.connection = state
	if state != Connected {
		d.colonyID = ""
	}
	d.mu.Unlock()
}

func (d *Daemon) triggerReconnect() {
	select {
	case d.reconnect <- struct{}{}:
	default:
	}
}

// zoneUsageLocked sums the live draw of every running stage worker. Callers
// hold d.mu.
func (d *Daemon) zoneUsageLocked() worker.Usage {
	var total worker.Usage
	for _, w := range d.workers {
		total.CPUCores += w.usage.CPUCores
		total.RAMGB += w.usage.RAMGB
	}
	return total
}

// handleCommand parses a serve command from the heartbeat and, unless that
// stage is already running here, launches its worker.
func (d *Daemon) handleCommand(ctx context.Context, raw string) {
	if strings.TrimSpace(raw) == "" {
		return
	}
	var cmd worker.Command
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		d.log("WARN", "Ignoring an unreadable command from the Coordinator")
		return
	}
	if cmd.DeploymentID == "" {
		return
	}

	key := cmd.DeploymentID + ":" + cmd.Role
	// A heartbeat stream is only a delivery channel. It can be recycled by a
	// tunnel or gRPC while a model is still loading, so the worker must not
	// inherit that stream's cancellation; the relay owns its lifecycle, and a
	// pause stops it through cancel.
	workerCtx, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	if _, running := d.workers[key]; running {
		d.mu.Unlock()
		cancel()
		return
	}
	d.workers[key] = &runningWorker{cancel: cancel, model: cmd.Model}
	host := coordinatorHost(d.cfg.CoordinatorURL)
	d.mu.Unlock()

	go d.runWorker(workerCtx, key, cmd, host)
}

func (d *Daemon) runWorker(ctx context.Context, key string, cmd worker.Command, host string) {
	defer d.recoverCrash("runWorker")
	d.log("INFO", fmt.Sprintf("Serving stage %d of %d for a Zone (deployment %s)", cmd.Stage+1, cmd.Stages, cmd.DeploymentID))

	sample := func(u worker.Usage) {
		d.mu.Lock()
		if w := d.workers[key]; w != nil {
			w.usage = u
		}
		d.mu.Unlock()
	}
	download := func(p worker.Download) {
		finished := p.Done >= p.Total
		d.mu.Lock()
		if w := d.workers[key]; w != nil {
			started := w.download == nil
			w.download = &p
			if finished {
				w.download = nil
			}
			if started && !finished {
				defer d.log("INFO", fmt.Sprintf("Downloading %s of %s for a Zone", megabytes(p.Total), p.Model))
			}
		}
		d.mu.Unlock()
		if finished {
			d.log("INFO", "Downloaded "+p.Model)
			d.fitCache()
		}
	}
	if d.cache != nil && cmd.Engine == "real" {
		d.cache.Touch(cmd.Model, time.Now())
		d.fitCache() // make room before this stage downloads anything
	}
	err := worker.Launch(ctx, cmd, host, d.log, sample, download)

	d.mu.Lock()
	delete(d.workers, key)
	d.mu.Unlock()
	if d.cache != nil && cmd.Engine == "real" {
		d.cache.Touch(cmd.Model, time.Now())
		d.fitCache()
	}

	switch {
	case ctx.Err() != nil:
		d.log("INFO", "Stopped the stage worker because this machine was paused")
		// Tell the Coordinator now, so the Zone's requests fail with the real
		// reason instead of waiting out the relay's reconnect grace period.
		d.PushError("WARN", "Zone deployment "+cmd.DeploymentID+" failed: its contributor paused a machine serving stage "+strconv.Itoa(cmd.Stage+1))
	case err != nil:
		d.PushError("ERROR", "Zone deployment "+cmd.DeploymentID+" failed: "+err.Error())
	}
}

// Shutdown stops every stage worker, for when the app quits. Workers do not
// inherit the daemon's context (a recycled heartbeat stream must not kill
// them), so quitting has to stop them explicitly.
func (d *Daemon) Shutdown() {
	d.stopWorkers()
}

// stopWorkers ends every stage worker on this machine, for example when the
// contributor pauses. The Zone's deployment then fails with a clear reason
// instead of waiting on a machine that is no longer contributing.
func (d *Daemon) stopWorkers() {
	d.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(d.workers))
	for _, w := range d.workers {
		cancels = append(cancels, w.cancel)
	}
	d.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func coordinatorHost(coordinatorURL string) string {
	return client.Host(coordinatorURL)
}

// PushError records an issue locally and forwards it to the Coordinator issues
// feed, so the operator sees node side problems. Without a live connection
// (for example right after a pause) it opens a short one of its own, since
// some reports, like a stage worker stopping, matter most at exactly that
// moment.
func (d *Daemon) PushError(level, message string) {
	d.log(level, message)

	d.mu.RLock()
	conn := d.conn
	addr := d.cfg.CoordinatorURL
	d.mu.RUnlock()
	creds := d.credentials()
	if creds == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), registerTTL)
		defer cancel()
		cli := conn
		if cli == nil {
			fresh, err := client.Dial(addr, creds)
			if err != nil {
				return
			}
			defer fresh.Close()
			cli = fresh
		}
		if err := cli.ReportError(ctx, creds.NodeID, level, message); err != nil {
			d.log("WARN", "Could not send the report to the Coordinator: "+err.Error())
		}
	}()
}

// recoverCrash catches a panic in the named goroutine so one bad job or a
// flaky driver call cannot take down the whole node process. It reports an
// anonymized crash report to the Coordinator issues feed: what kind of error
// it was, where it happened in the code, and the operating system. It never
// includes the hostname, username, file paths outside this repo, or any
// network address, so the report cannot identify the contributor or their
// machine.
func (d *Daemon) recoverCrash(where string) {
	r := recover()
	if r == nil {
		return
	}
	d.mu.RLock()
	enabled := d.cfg.CrashReportsEnabled
	osName := d.specs.OS
	d.mu.RUnlock()

	trace := string(debug.Stack())
	d.log("ERROR", fmt.Sprintf("Recovered from a crash in %s: %v", where, r))

	if !enabled {
		return
	}
	report := fmt.Sprintf("Crash in %s (%T): %v\nOS: %s\n%s", where, r, r, osName, trace)
	d.PushError("ERROR", report)
}

// checkGPUTemp warns the operator once when the GPU crosses the temperature
// threshold, and notes when it recovers. Debounced so it does not spam.
func (d *Daemon) checkGPUTemp(tempC float64) {
	if tempC <= 0 {
		return
	}
	d.mu.Lock()
	over := d.gpuOverTemp
	d.mu.Unlock()

	switch {
	case tempC >= gpuWarnTempC && !over:
		d.mu.Lock()
		d.gpuOverTemp = true
		d.mu.Unlock()
		d.PushError("WARN", "GPU temperature is high: "+itoa(int(tempC))+" C")
	case tempC < gpuWarnTempC && over:
		d.mu.Lock()
		d.gpuOverTemp = false
		d.mu.Unlock()
		d.log("INFO", "GPU temperature back to normal: "+itoa(int(tempC))+" C")
	}
}

func (d *Daemon) log(level, message string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, Event{Time: time.Now().UTC(), Level: level, Message: message})
	if len(d.events) > maxEvents {
		d.events = d.events[len(d.events)-maxEvents:]
	}
}

func clampAllocation(a config.Allocation, s resources.Specs) config.Allocation {
	a.CPUCores = clamp(a.CPUCores, 0, s.CPUCores)
	a.RAMGB = clamp(a.RAMGB, 0, s.RAMGB)
	a.GPUMemory = clamp(a.GPUMemory, 0, s.GPUMemoryGB)
	if a.BandwidthMbps < 0 {
		a.BandwidthMbps = 0
	}
	return a
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if hi > 0 && v > hi {
		return hi
	}
	return v
}

func humanSpecs(s resources.Specs) string {
	out := itoa(s.CPUCores) + " cores, " + itoa(s.RAMGB) + " GB RAM"
	if s.GPUModel != "" {
		out += ", " + s.GPUModel
		if s.GPUMemoryGB > 0 {
			out += " " + itoa(s.GPUMemoryGB) + " GB"
		}
	}
	return out
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func nextBackoff(b time.Duration) time.Duration {
	b *= 2
	if b > maxBackoff {
		return maxBackoff
	}
	return b
}
