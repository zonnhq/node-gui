// Package idle tells how long the machine's owner has been away (no keyboard
// or mouse input), so a node with "Only when idle" contributes only while its
// owner is not using the machine.
package idle

import (
	"errors"
	"fmt"
	"time"
)

// Source reports how long it has been since the last keyboard or mouse input.
type Source interface {
	Idle() (time.Duration, error)
}

// ErrUnsupported means this machine offers no way to read the idle time, for
// example a Linux desktop without an idle service, or a session with no
// display at all.
var ErrUnsupported = errors.New("this desktop does not report how long it has been idle")

// System returns the idle source for this operating system.
func System() Source { return systemSource() }

// Decision is whether the node may contribute now, with a sentence for the
// contributor when it may not.
type Decision struct {
	Allowed bool
	Note    string
}

// Decide applies "Only when idle": with it off, the node always may; with it
// on, only after the owner has been away for at least threshold. When the idle
// time cannot be read, the node does not contribute, since the owner asked
// not to be disturbed and the node cannot tell.
func Decide(onlyWhenIdle bool, threshold time.Duration, idle time.Duration, err error) Decision {
	if !onlyWhenIdle {
		return Decision{Allowed: true}
	}
	if err != nil {
		return Decision{Note: "Only when idle is on, but this computer does not report how long it has been idle, so Zonn Node is not contributing. Turn Only when idle off to contribute."}
	}
	if idle >= threshold {
		return Decision{Allowed: true}
	}
	return Decision{Note: fmt.Sprintf("Waiting until you have been away for %s (Only when idle is on)", humanMinutes(threshold))}
}

func humanMinutes(d time.Duration) string {
	m := int(d.Round(time.Minute) / time.Minute)
	if m == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", m)
}
