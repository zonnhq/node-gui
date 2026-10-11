//go:build linux

package worker

import (
	"os/exec"
	"syscall"
)

// setParentDeathSignal makes Linux kill the worker if the node process dies,
// even when it is killed outright and cannot stop the worker itself.
func setParentDeathSignal(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
