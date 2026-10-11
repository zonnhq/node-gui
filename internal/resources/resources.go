// Package resources detects the machine's hardware and samples live usage.
// CPU, RAM, and disk come from gopsutil. GPU comes from nvidia-smi on NVIDIA
// hardware and system_profiler on macOS. GPU detection is best effort: a
// machine with no supported GPU simply reports an empty model.
package resources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

// Specs is the detected physical hardware.
type Specs struct {
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	CPUCores    int    `json:"cpu_cores"`
	RAMGB       int    `json:"ram_gb"`
	DiskGB      int    `json:"disk_gb"`
	GPUModel    string `json:"gpu_model"`
	GPUMemoryGB int    `json:"gpu_memory_gb"`
}

// Utilization is a single live sample.
type Utilization struct {
	CPUUsedCores float64 `json:"cpu_used"`
	RAMUsedGB    float64 `json:"ram_used_gb"`
	GPUMemUsedGB float64 `json:"gpu_mem_used_gb"`
	GPUTempC     float64 `json:"gpu_temp_c"`
}

const bytesPerGB = 1024 * 1024 * 1024

// Detect gathers the machine specs. Individual failures degrade gracefully to
// zero values rather than failing the whole detection.
func Detect() Specs {
	s := Specs{Arch: runtime.GOARCH}

	if info, err := host.Info(); err == nil {
		s.OS = friendlyOS(info.Platform, info.PlatformVersion)
		if info.KernelArch != "" {
			s.Arch = info.KernelArch
		}
	} else {
		s.OS = runtime.GOOS
	}

	if cores, err := cpu.Counts(true); err == nil {
		s.CPUCores = cores
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		s.RAMGB = int(vm.Total / bytesPerGB)
	}
	if usage, err := disk.Usage(rootPath()); err == nil {
		s.DiskGB = int(usage.Total / bytesPerGB)
	}

	model, gpuMem := detectGPU()
	s.GPUModel = model
	s.GPUMemoryGB = gpuMem

	return s
}

// Fingerprint returns a stable identifier for this physical machine, derived
// from the host machine id, hostname, and architecture. The Coordinator uses it
// to recognise the same device across reinstalls so it never appears as two
// nodes. It is hashed so the raw machine id never leaves the box.
func Fingerprint() string {
	// An explicit override lets several nodes run on one machine (local testing,
	// or a deliberate multi-tenant host) without collapsing into one record.
	if v := strings.TrimSpace(os.Getenv("COLONY_FINGERPRINT")); v != "" {
		return v
	}
	parts := []string{runtime.GOARCH}
	if info, err := host.Info(); err == nil {
		parts = append(parts, info.HostID, info.Hostname)
	}
	joined := strings.TrimSpace(strings.Join(parts, "|"))
	if joined == "" || joined == runtime.GOARCH {
		return "" // nothing stable to fingerprint; let the Coordinator use the node id
	}
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:16])
}

// Sample takes a live utilization reading. The CPU percentage is measured over
// a short window and converted to a used-cores figure.
func Sample(totalCores int) Utilization {
	var u Utilization

	if pcts, err := cpu.Percent(200*time.Millisecond, false); err == nil && len(pcts) > 0 {
		u.CPUUsedCores = pcts[0] / 100.0 * float64(totalCores)
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		u.RAMUsedGB = float64(vm.Used) / bytesPerGB
	}
	gpuMemUsed, gpuTemp := sampleGPU()
	u.GPUMemUsedGB = gpuMemUsed
	u.GPUTempC = gpuTemp

	return u
}

func friendlyOS(platform, version string) string {
	platform = strings.TrimSpace(platform)
	version = strings.TrimSpace(version)
	if platform == "" {
		return runtime.GOOS
	}
	if version == "" {
		return platform
	}
	return platform + " " + version
}

func rootPath() string {
	if runtime.GOOS == "windows" {
		return "C:\\"
	}
	return "/"
}

// detectGPU returns the primary GPU model and its total memory in GB.
func detectGPU() (string, int) {
	switch runtime.GOOS {
	case "darwin":
		return detectGPUDarwin()
	default:
		return detectGPUNvidia()
	}
}

func detectGPUNvidia() (string, int) {
	out, err := runCmd(2*time.Second, "nvidia-smi",
		"--query-gpu=name,memory.total", "--format=csv,noheader,nounits")
	if err != nil {
		return "", 0
	}
	line := firstLine(out)
	if line == "" {
		return "", 0
	}
	parts := strings.Split(line, ",")
	if len(parts) < 2 {
		return strings.TrimSpace(line), 0
	}
	name := strings.TrimSpace(parts[0])
	memMB, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	return name, memMB / 1024
}

func detectGPUDarwin() (string, int) {
	out, err := runCmd(4*time.Second, "system_profiler", "SPDisplaysDataType")
	if err != nil {
		return "", 0
	}
	var model string
	var vram int
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "Chipset Model:") {
			model = strings.TrimSpace(strings.TrimPrefix(line, "Chipset Model:"))
		}
		if strings.HasPrefix(line, "VRAM") {
			if idx := strings.Index(line, ":"); idx >= 0 {
				fields := strings.Fields(line[idx+1:])
				if len(fields) > 0 {
					if n, err := strconv.Atoi(fields[0]); err == nil {
						vram = n / 1024 // reported in MB
					}
				}
			}
		}
	}
	return model, vram
}

// sampleGPU returns used GPU memory in GB and temperature in Celsius, NVIDIA only.
func sampleGPU() (float64, float64) {
	out, err := runCmd(2*time.Second, "nvidia-smi",
		"--query-gpu=memory.used,temperature.gpu", "--format=csv,noheader,nounits")
	if err != nil {
		return 0, 0
	}
	line := firstLine(out)
	parts := strings.Split(line, ",")
	if len(parts) < 2 {
		return 0, 0
	}
	memMB, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	temp, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	return memMB / 1024.0, temp
}

func runCmd(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
