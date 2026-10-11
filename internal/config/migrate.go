package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
)

const (
	dirName       = ".zonn"
	legacyDirName = ".colony" // the folder's name before the rename to Zonn
)

var (
	migrateOnce sync.Once
	resolvedDir string
)

// defaultDir returns ~/.zonn, first moving a ~/.colony folder from an older
// version there, once per process. If the move fails, it keeps using
// ~/.colony so the node still starts with its settings and credential.
func defaultDir(home string) string {
	migrateOnce.Do(func() {
		dir, err := migrateDir(filepath.Join(home, legacyDirName), filepath.Join(home, dirName))
		if err != nil {
			log.Printf("config: could not move %s to %s, still using the old folder: %v", legacyDirName, dirName, err)
		}
		resolvedDir = dir
	})
	return resolvedDir
}

// migrateDir moves the folder old to dir and returns the folder to use. It
// copies everything (keeping file permissions, so credentials stay readable
// only by the user), checks every copied file, swaps the copy into place in
// one rename, and only then removes old. Nothing happens when dir already
// exists or old does not. On any failure it returns old, untouched.
func migrateDir(old, dir string) (string, error) {
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return dir, err
	}
	if info, err := os.Stat(old); err != nil || !info.IsDir() {
		return dir, nil
	}
	staging := dir + ".moving"
	if err := os.RemoveAll(staging); err != nil {
		return old, err
	}
	if err := copyTree(old, staging); err != nil {
		os.RemoveAll(staging)
		return old, fmt.Errorf("copy: %w", err)
	}
	if err := sameTree(old, staging); err != nil {
		os.RemoveAll(staging)
		return old, fmt.Errorf("verify: %w", err)
	}
	if err := os.Rename(staging, dir); err != nil {
		os.RemoveAll(staging)
		return old, fmt.Errorf("rename: %w", err)
	}
	if err := os.RemoveAll(old); err != nil {
		// The new folder is complete; a leftover old one is harmless.
		log.Printf("config: moved settings to %s but could not remove %s: %v", dir, old, err)
	} else {
		log.Printf("config: moved settings from %s to %s", old, dir)
	}
	return dir, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, info.Mode().Perm())
		default:
			return nil // links and devices are not part of the node's data
		}
	})
}

// sameTree checks that every regular file under src exists under dst with
// the same contents.
func sameTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		want, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil {
			return err
		}
		if !bytes.Equal(want, got) {
			return fmt.Errorf("%s differs after copying", rel)
		}
		return nil
	})
}
