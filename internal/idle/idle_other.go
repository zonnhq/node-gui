//go:build !linux && !darwin && !windows

package idle

import "time"

type noSource struct{}

func systemSource() Source { return noSource{} }

func (noSource) Idle() (time.Duration, error) { return 0, ErrUnsupported }
