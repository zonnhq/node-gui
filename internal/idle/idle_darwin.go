package idle

import (
	"os/exec"
	"time"
)

// darwinSource reads HIDIdleTime, the nanoseconds since the last input, from
// the IOHIDSystem registry entry.
type darwinSource struct{}

func systemSource() Source { return darwinSource{} }

func (darwinSource) Idle() (time.Duration, error) {
	out, err := exec.Command("ioreg", "-c", "IOHIDSystem", "-d", "4").Output()
	if err != nil {
		return 0, ErrUnsupported
	}
	return parseHIDIdleTime(out)
}
