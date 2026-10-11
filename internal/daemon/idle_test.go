package daemon

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zonnhq/node-gui/internal/config"
	"github.com/zonnhq/node-gui/internal/idle"
)

type fakeIdle struct {
	mu   sync.Mutex
	away time.Duration
	err  error
}

func (f *fakeIdle) Idle() (time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.away, f.err
}

func (f *fakeIdle) set(away time.Duration, err error) {
	f.mu.Lock()
	f.away, f.err = away, err
	f.mu.Unlock()
}

func drained(d *Daemon) bool {
	select {
	case <-d.reconnect:
		return true
	default:
		return false
	}
}

func TestOnlyWhenIdleHoldsTheNodeBackWhileTheOwnerIsActive(t *testing.T) {
	t.Setenv("ZONN_HOME", t.TempDir())
	cfg := config.Default()
	cfg.Configured = true
	cfg.OnlyWhenIdle = true
	cfg.IdleMinutes = 5
	d := New(cfg)
	src := &fakeIdle{away: time.Minute}
	d.SetIdleSource(src)

	if dec := d.checkIdle(); dec.Allowed || d.available() {
		t.Fatal("contributing while the owner is active")
	}
	if !drained(d) {
		t.Fatal("the run loop was not woken to pause")
	}
	st := d.Snapshot()
	if !st.WaitingForIdle || !strings.Contains(st.IdleNote, "5 minutes") {
		t.Fatalf("state: waiting=%v note=%q", st.WaitingForIdle, st.IdleNote)
	}

	// No change, no wake up: an unchanged decision must not restart the
	// Coordinator session every few seconds.
	d.checkIdle()
	if drained(d) {
		t.Fatal("woke the run loop without a change")
	}

	src.set(6*time.Minute, nil)
	if dec := d.checkIdle(); !dec.Allowed || !d.available() || !drained(d) {
		t.Fatal("did not resume after the owner was away long enough")
	}

	// The owner comes back: paused again at once.
	src.set(2*time.Second, nil)
	if d.checkIdle(); d.available() {
		t.Fatal("kept contributing after the owner came back")
	}

	// Turning the setting off releases the node immediately.
	off := d.Config()
	off.OnlyWhenIdle = false
	if err := d.UpdateConfig(off); err != nil {
		t.Fatal(err)
	}
	if !d.available() || d.Snapshot().WaitingForIdle {
		t.Fatal("still held back after Only when idle was turned off")
	}
}

func TestOnlyWhenIdleDoesNotContributeWhenIdleTimeIsUnknown(t *testing.T) {
	t.Setenv("ZONN_HOME", t.TempDir())
	cfg := config.Default()
	cfg.Configured = true
	cfg.OnlyWhenIdle = true
	d := New(cfg)
	d.SetIdleSource(&fakeIdle{err: idle.ErrUnsupported})
	d.checkIdle()
	if d.available() || !strings.Contains(d.Snapshot().IdleNote, "does not report") {
		t.Fatalf("unknown idle time: available=%v note=%q", d.available(), d.Snapshot().IdleNote)
	}
}

func TestIdleMinutesAreBounded(t *testing.T) {
	t.Setenv("ZONN_HOME", t.TempDir())
	cfg := config.Default()
	cfg.Configured = true
	d := New(cfg)
	for in, want := range map[int]int{0: config.DefaultIdleMinutes, 500: config.MaxIdleMinutes, 7: 7} {
		c := d.Config()
		c.IdleMinutes = in
		if err := d.UpdateConfig(c); err != nil {
			t.Fatal(err)
		}
		if got := d.Config().IdleMinutes; got != want {
			t.Fatalf("idle minutes %d became %d, want %d", in, got, want)
		}
	}
}
