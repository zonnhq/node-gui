package idle

import (
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// linuxSource asks the desktop over D-Bus: GNOME's Mutter idle monitor
// (X11 and Wayland), then the freedesktop screensaver service (KDE and
// others), then the xprintidle tool on X11.
type linuxSource struct{}

func systemSource() Source { return linuxSource{} }

func (linuxSource) Idle() (time.Duration, error) {
	if conn, err := dbus.SessionBus(); err == nil {
		var ms uint64
		if err := conn.Object("org.gnome.Mutter.IdleMonitor", "/org/gnome/Mutter/IdleMonitor/Core").
			Call("org.gnome.Mutter.IdleMonitor.GetIdletime", 0).Store(&ms); err == nil {
			return time.Duration(ms) * time.Millisecond, nil
		}
		var ms32 uint32
		if err := conn.Object("org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver").
			Call("org.freedesktop.ScreenSaver.GetSessionIdleTime", 0).Store(&ms32); err == nil {
			return time.Duration(ms32) * time.Millisecond, nil
		}
	}
	if out, err := exec.Command("xprintidle").Output(); err == nil {
		if ms, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64); err == nil {
			return time.Duration(ms) * time.Millisecond, nil
		}
	}
	return 0, ErrUnsupported
}
