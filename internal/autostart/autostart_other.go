//go:build !linux && !darwin && !windows

package autostart

import "errors"

func enable(string) error { return errors.New("starting at sign in is not supported on this system") }
func disable() error      { return nil }
func enabled() bool       { return false }
