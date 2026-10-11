// Package updater downloads a newer Zonn Node installer from the backend,
// checks it against the SHA-256 the backend published, and opens it so the
// contributor can install it. A download that does not match is deleted and
// never opened.
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zonnhq/node-gui/internal/version"
)

// Platform is what this build is, for choosing its installer.
type Platform struct {
	OS, Arch, Variant, Format string
}

// Current returns this build's platform.
func Current() Platform {
	variant := version.Variant
	if variant == "" {
		variant = "cpu"
	}
	return Platform{OS: runtime.GOOS, Arch: runtime.GOARCH, Variant: variant, Format: formatFor(runtime.GOOS)}
}

func formatFor(goos string) string {
	switch goos {
	case "windows":
		return "exe"
	case "darwin":
		return "dmg"
	default:
		return "deb"
	}
}

// ErrChecksum means the downloaded installer did not match its published
// SHA-256 and was deleted.
var ErrChecksum = errors.New("the downloaded installer did not match its published checksum, so it was deleted; try again")

// Downloader streams an installer from an offset.
type Downloader func(ctx context.Context, fileName string, offset int64, w io.Writer) error

// Fetch downloads fileName into dir, resuming a partial download, and checks
// the whole file against size and sha256. It returns the verified file's
// path. progress, if set, hears the bytes on disk so far.
func Fetch(ctx context.Context, dl Downloader, dir, fileName string, size int64, sha256Hex string, progress func(done int64)) (string, error) {
	if fileName != filepath.Base(fileName) || strings.HasPrefix(fileName, ".") || fileName == "" {
		return "", fmt.Errorf("invalid installer name %q", fileName)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	final := filepath.Join(dir, fileName)
	if ok, _ := matches(final, size, sha256Hex); ok {
		return final, nil // already downloaded and verified
	}
	part := final + ".part"
	offset := int64(0)
	if info, err := os.Stat(part); err == nil && info.Size() <= size {
		offset = info.Size()
	}
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	if err := f.Truncate(offset); err != nil {
		f.Close()
		return "", err
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		f.Close()
		return "", err
	}
	w := &counting{w: f, done: offset, progress: progress}
	if offset < size {
		err = dl(ctx, fileName, offset, w)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err // the partial file stays, to resume
	}
	ok, err := matches(part, size, sha256Hex)
	if err != nil {
		return "", err
	}
	if !ok {
		os.Remove(part)
		return "", ErrChecksum
	}
	if err := os.Rename(part, final); err != nil {
		return "", err
	}
	return final, nil
}

func matches(path string, size int64, sha256Hex string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return false, err
	}
	return n == size && strings.EqualFold(hex.EncodeToString(h.Sum(nil)), sha256Hex), nil
}

type counting struct {
	w        io.Writer
	done     int64
	progress func(int64)
}

func (c *counting) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.done += int64(n)
	if c.progress != nil {
		c.progress(c.done)
	}
	return n, err
}

// Open starts the installer the way the operating system would on a double
// click: Inno Setup on Windows (it closes the running app before replacing
// it), the disk image on macOS, and the package installer (App Center on
// Ubuntu) on Linux.
func Open(path string) error {
	name, args := openCommand(runtime.GOOS, path)
	cmd := exec.Command(name, args...)
	return cmd.Start()
}

func openCommand(goos, path string) (string, []string) {
	switch goos {
	case "windows":
		return path, nil
	case "darwin":
		return "open", []string{path}
	default:
		return "xdg-open", []string{path}
	}
}
