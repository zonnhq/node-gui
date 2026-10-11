package idle

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDecide(t *testing.T) {
	five := 5 * time.Minute
	if d := Decide(false, five, 0, errors.New("x")); !d.Allowed {
		t.Fatal("off must always allow")
	}
	if d := Decide(true, five, 4*time.Minute, nil); d.Allowed || !strings.Contains(d.Note, "5 minutes") {
		t.Fatalf("in use: %+v", d)
	}
	if d := Decide(true, five, five, nil); !d.Allowed {
		t.Fatal("away long enough must allow")
	}
	if d := Decide(true, five, time.Hour, ErrUnsupported); d.Allowed || !strings.Contains(d.Note, "does not report") {
		t.Fatalf("unknown idle time: %+v", d)
	}
	if d := Decide(true, time.Minute, 0, nil); !strings.Contains(d.Note, "1 minute") {
		t.Fatalf("singular: %q", d.Note)
	}
}

func TestParseHIDIdleTime(t *testing.T) {
	out := []byte(`    | |   "HIDIdleTime" = 2500000000
    | |   "HIDKeyboardModifierMappingPairs" = ()`)
	if d, err := parseHIDIdleTime(out); err != nil || d != 2500*time.Millisecond {
		t.Fatalf("%v %v", d, err)
	}
	if _, err := parseHIDIdleTime([]byte("nothing")); err == nil {
		t.Fatal("missing key accepted")
	}
}

func TestSystemSourceOnThisMachine(t *testing.T) {
	// Only checks that the call returns; CI machines have no desktop.
	if d, err := System().Idle(); err == nil && d < 0 {
		t.Fatalf("negative idle time %v", d)
	}
}
