// Package worker launches the inference worker for one pipeline stage of a
// Zone's deployment. The node receives a serve command from the Coordinator,
// then runs the worker as a long lived subprocess pointed at the Coordinator
// relay WebSocket, until the deployment ends or the contributor pauses.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/zonnhq/node-gui/internal/config"
)

const bytesPerGB = 1024 * 1024 * 1024

// Command mirrors the job spec the Coordinator sends in the heartbeat response.
type Command struct {
	DeploymentID string  `json:"deployment_id"`
	Role         string  `json:"role"` // relay role, "stage0" to "stageN-1"
	Stage        int     `json:"stage"`
	Stages       int     `json:"stages"`
	LayerStart   float64 `json:"layer_start"`
	LayerEnd     float64 `json:"layer_end"`
	Model        string  `json:"model"`
	Revision     string  `json:"model_revision"` // pinned catalog revision
	Engine       string  `json:"engine"`
	Device       string  `json:"device"` // cpu, or auto to use the GPU
	RelayPort    string  `json:"relay_port"`
	RelayPath    string  `json:"relay_path"`
	RelayURL     string  `json:"relay_url"`
	RelayToken   string  `json:"relay_token"`
}

// scriptPath resolves the worker script from an explicit override, the node's
// data directory, or next to the released desktop binary.
func scriptPath() (string, error) {
	if s := config.Env("ZONN_WORKER_SCRIPT"); s != "" {
		if _, err := os.Stat(s); err != nil {
			return "", fmt.Errorf("worker script not found at %s", s)
		}
		return s, nil
	}

	executable, _ := os.Executable()
	return findScriptPath(executable, runtime.GOOS)
}

func findScriptPath(executable, goos string) (string, error) {
	var candidates []string
	if dataDir, err := config.Dir(); err == nil {
		candidates = append(candidates, filepath.Join(dataDir, "inference_worker.py"))
	}
	if executable != "" {
		appDir := filepath.Dir(executable)
		candidates = append(candidates, filepath.Join(appDir, "inference_worker.py"))
		if goos == "darwin" {
			candidates = append(candidates, filepath.Clean(filepath.Join(appDir, "..", "Resources", "inference_worker.py")))
		}
	}
	if goos == "linux" {
		candidates = append(candidates, "/usr/lib/zonn/inference_worker.py")
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("worker script not found; place inference_worker.py in the Zonn Node folder or set ZONN_WORKER_SCRIPT")
}

func pythonCommand() (string, []string, error) {
	if p := config.Env("ZONN_PYTHON"); p != "" {
		return p, nil, nil
	}
	candidates := []struct {
		name string
		args []string
	}{
		{name: "python3"},
		{name: "python"},
	}
	if runtime.GOOS == "windows" {
		candidates = []struct {
			name string
			args []string
		}{
			{name: "py", args: []string{"-3"}},
			{name: "python"},
			{name: "python3"},
		}
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate.name); err == nil {
			return path, candidate.args, nil
		}
	}
	return "", nil, fmt.Errorf("Python 3 was not found; install Python 3 or set ZONN_PYTHON")
}

func bundledWorkerPath(appExecutable, goos string) (string, error) {
	if configured := config.Env("ZONN_WORKER_EXECUTABLE"); configured != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured, nil
		}
		return "", fmt.Errorf("bundled worker not found at %s", configured)
	}

	name := "inference-worker"
	if goos == "windows" {
		name += ".exe"
	}
	// The standard runner is one file named inference-worker. The NVIDIA
	// runner is a folder of that name holding the executable and its CUDA
	// libraries, so each place is checked for both.
	var dirs []string
	if appExecutable != "" {
		appDir := filepath.Dir(appExecutable)
		dirs = append(dirs, appDir)
		if goos == "darwin" {
			dirs = append(dirs, filepath.Clean(filepath.Join(appDir, "..", "Resources")))
		}
	}
	if goos == "linux" {
		dirs = append(dirs, "/usr/lib/zonn")
	}
	var candidates []string
	for _, dir := range dirs {
		candidates = append(candidates, filepath.Join(dir, name), filepath.Join(dir, "inference-worker", name))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}

func workerCommand(engine string) (string, []string, error) {
	executable, _ := os.Executable()
	if bundled, err := bundledWorkerPath(executable, runtime.GOOS); err == nil {
		return bundled, nil, nil
	}

	script, err := scriptPath()
	if err != nil {
		return "", nil, fmt.Errorf("bundled inference worker is missing and Python fallback is unavailable: %w", err)
	}
	python, pythonArgs, err := pythonCommand()
	if err != nil {
		return "", nil, err
	}
	return python, append(pythonArgs, script), nil
}

// Usage is the worker process's live resource draw, which is how much of the
// contribution the Zone is actually using on this node.
type Usage struct {
	CPUCores float64
	RAMGB    float64
}

// Launch runs the worker for one job and blocks until it exits. Output is passed
// to logfn line by line. While the worker runs, sampleFn is called about once a
// second with the process's live CPU and memory draw, so the node can show what
// of the contribution the Zone is using; it is called with a zero Usage once
// the worker exits. sampleFn may be nil.
func Launch(ctx context.Context, cmd Command, coordinatorHost string, logfn func(level, message string), sampleFn func(Usage)) error {
	relayPath := cmd.RelayPath
	if relayPath == "" {
		relayPath = "/relay"
	}
	wsURL, err := commandRelayURL(cmd, coordinatorHost, relayPath)
	if err != nil {
		return err
	}

	args := workerArgs(cmd, wsURL)
	logfn("INFO", fmt.Sprintf("Starting the stage %d of %d worker for Zone deployment %s", cmd.Stage+1, cmd.Stages, cmd.DeploymentID))

	var out bytes.Buffer
	program, prefixArgs, err := workerCommand(cmd.Engine)
	if err != nil {
		return err
	}
	proc := exec.CommandContext(ctx, program, append(prefixArgs, args...)...)
	// The relay token goes through the environment, not the command line, so
	// other users on the machine cannot read it from the process list.
	proc.Env = append(os.Environ(),
		"HF_HUB_DISABLE_SYMLINKS_WARNING=1",
		"HF_HUB_DISABLE_XET=1",
		"ZONN_RELAY_TOKEN="+cmd.RelayToken,
	)
	proc.Stdout = &out
	proc.Stderr = &out
	setParentDeathSignal(proc)
	if err := proc.Start(); err != nil {
		return fmt.Errorf("start worker: %w", err)
	}

	// Sample the worker's usage until it exits, then reset to zero.
	done := make(chan struct{})
	if sampleFn != nil {
		go sampleUsage(proc.Process.Pid, sampleFn, done)
	}

	waitErr := proc.Wait()
	close(done)
	if sampleFn != nil {
		sampleFn(Usage{})
	}

	// out is only written by the process, and it has now exited, so reading it
	// here is race free.
	for _, line := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			logfn("INFO", "worker: "+line)
		}
	}
	if waitErr != nil {
		detail := lastWorkerError(out.String())
		if detail != "" {
			return fmt.Errorf("worker exited: %w: %s", waitErr, detail)
		}
		return fmt.Errorf("worker exited: %w", waitErr)
	}
	logfn("INFO", fmt.Sprintf("Worker for Zone deployment %s finished", cmd.DeploymentID))
	return nil
}

func workerArgs(cmd Command, wsURL string) []string {
	args := []string{
		"--stage", strconv.Itoa(cmd.Stage),
		"--stages", strconv.Itoa(cmd.Stages),
		"--layer-start", strconv.FormatFloat(cmd.LayerStart, 'f', -1, 64),
		"--layer-end", strconv.FormatFloat(cmd.LayerEnd, 'f', -1, 64),
		"--relay-ws", wsURL,
		"--engine", cmd.Engine,
		"--model", cmd.Model,
	}
	if cmd.Revision != "" {
		args = append(args, "--revision", cmd.Revision)
	}
	device := cmd.Device
	if device != "auto" {
		device = "cpu" // the GPU is used only when the Coordinator planned for it
	}
	args = append(args, "--device", device)
	return args
}

// Accelerator is what the inference runtime can drive.
type Accelerator struct {
	Device   string  `json:"device"` // cuda, mps, or cpu
	Name     string  `json:"name"`
	MemoryGB float64 `json:"memory_gb"`
	Error    string  `json:"error"`
}

// Probe asks the worker runtime which accelerator it can use. Importing torch
// takes a while, so callers run it in the background.
func Probe(ctx context.Context) (Accelerator, error) {
	program, prefix, err := workerCommand("real")
	if err != nil {
		return Accelerator{}, err
	}
	out, err := exec.CommandContext(ctx, program, append(prefix, "--probe")...).Output()
	if err != nil {
		return Accelerator{}, err
	}
	var acc Accelerator
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &acc); err != nil {
		return Accelerator{}, fmt.Errorf("unreadable probe output: %w", err)
	}
	return acc, nil
}

func lastWorkerError(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(strings.TrimSuffix(lines[i], "\r"))
		if line == "" || strings.Contains(line, "Loading weights:") {
			continue
		}
		if len(line) > 500 {
			line = line[:500]
		}
		return line
	}
	return ""
}

func commandRelayURL(cmd Command, coordinatorHost, relayPath string) (string, error) {
	raw := cmd.RelayURL
	if raw == "" {
		raw = fmt.Sprintf("ws://%s:%s%s", coordinatorHost, cmd.RelayPort, relayPath)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" {
		return "", fmt.Errorf("invalid relay URL %q", raw)
	}
	query := u.Query()
	query.Set("deployment", cmd.DeploymentID)
	query.Set("role", cmd.Role)
	u.RawQuery = query.Encode()
	return u.String(), nil
}

// sampleUsage reports the worker process's CPU and memory draw about once a
// second until done is closed. gopsutil Percent needs a prior call to establish
// a baseline, so the first tick is primed and skipped.
func sampleUsage(pid int, sampleFn func(Usage), done chan struct{}) {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return
	}
	_, _ = p.Percent(0) // prime the CPU baseline
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			var u Usage
			if pct, err := p.Percent(0); err == nil {
				u.CPUCores = pct / 100.0 // gopsutil reports 100 percent as one core
			}
			if mi, err := p.MemoryInfo(); err == nil && mi != nil {
				u.RAMGB = float64(mi.RSS) / bytesPerGB
			}
			sampleFn(u)
		}
	}
}
