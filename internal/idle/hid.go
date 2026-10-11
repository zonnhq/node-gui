package idle

import (
	"bytes"
	"strconv"
	"time"
)

// parseHIDIdleTime finds `"HIDIdleTime" = <nanoseconds>` in ioreg output.
func parseHIDIdleTime(out []byte) (time.Duration, error) {
	key := []byte(`"HIDIdleTime" = `)
	i := bytes.Index(out, key)
	if i < 0 {
		return 0, ErrUnsupported
	}
	rest := out[i+len(key):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	ns, err := strconv.ParseInt(string(rest[:end]), 10, 64)
	if err != nil {
		return 0, ErrUnsupported
	}
	return time.Duration(ns), nil
}
