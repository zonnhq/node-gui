//go:build !linux

package worker

import "os/exec"

// setParentDeathSignal has no portable equivalent outside Linux; there the
// node stops its workers when it shuts down normally.
func setParentDeathSignal(cmd *exec.Cmd) {}
