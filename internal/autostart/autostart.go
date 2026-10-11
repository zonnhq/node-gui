// Package autostart starts Zonn Node when the user signs in, hidden in the
// background: an XDG autostart entry on Linux, a LaunchAgent on macOS, and a
// Run key under the current user on Windows. Nothing needs administrator
// rights, and turning it off removes exactly what turning it on added.
package autostart

// HiddenFlag is the argument the sign in entry passes, so the app starts
// without opening its window.
const HiddenFlag = "--hidden"

// Set turns starting at sign in on (for the executable at exe) or off.
func Set(on bool, exe string) error {
	if on {
		return enable(exe)
	}
	return disable()
}

// Enabled reports whether starting at sign in is set up.
func Enabled() bool { return enabled() }
