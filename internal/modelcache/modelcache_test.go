package modelcache

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// put writes a fake model of n bytes in the Hugging Face layout: a blob and a
// snapshot link to it.
func put(t *testing.T, dir, name string, n int) {
	t.Helper()
	base := filepath.Join(dir, folder(name))
	blob := filepath.Join(base, "blobs", "abc")
	snap := filepath.Join(base, "snapshots", "rev")
	for _, d := range []string{filepath.Dir(blob), snap} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(blob, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(blob, filepath.Join(snap, "model.safetensors")); err != nil {
			t.Fatal(err)
		}
	}
}

func names(models []Model) []string {
	var out []string
	for _, m := range models {
		out = append(out, m.Name)
	}
	return out
}

func TestListCountsBlobsOnceAndOrdersByLastUse(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)
	put(t, dir, "org/old", 300)
	put(t, dir, "org/new", 500)
	now := time.Now()
	c.Touch("org/old", now.Add(-time.Hour))
	c.Touch("org/new", now)
	models := c.List(map[string]bool{"org/new": true})
	if len(models) != 2 || models[0].Name != "org/old" || models[0].Bytes != 300 || models[1].Bytes != 500 || !models[1].InUse || models[0].InUse {
		t.Fatalf("models: %+v", models)
	}
}

func TestFitRemovesTheLeastRecentlyUsedButNeverAModelInUse(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)
	now := time.Now()
	for i, name := range []string{"org/a", "org/b", "org/c"} {
		put(t, dir, name, 400)
		c.Touch(name, now.Add(time.Duration(i)*time.Minute))
	}
	if err := os.MkdirAll(filepath.Join(dir, ".locks", folder("org/a")), 0o755); err != nil {
		t.Fatal(err)
	}
	// 1200 bytes against a cap of 500: a is the oldest but in use, so b goes,
	// then c; a stays even though the cache is still over the cap.
	removed := c.Fit(500, map[string]bool{"org/a": true})
	if got := names(removed); len(got) != 2 || got[0] != "org/b" || got[1] != "org/c" {
		t.Fatalf("removed %v", got)
	}
	if got := names(c.List(nil)); len(got) != 1 || got[0] != "org/a" {
		t.Fatalf("left %v", got)
	}
	// No cap, nothing removed.
	if removed := c.Fit(0, nil); removed != nil {
		t.Fatalf("removed with no cap: %v", removed)
	}
	// Under the cap, nothing removed.
	if removed := c.Fit(10_000, nil); len(removed) != 0 {
		t.Fatalf("removed under the cap: %v", removed)
	}
}

func TestClearKeepsModelsInUseAndLeavesOtherFilesAlone(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)
	put(t, dir, "org/a", 10)
	put(t, dir, "org/b", 10)
	other := filepath.Join(dir, "not-a-model.txt")
	os.WriteFile(other, []byte("x"), 0o644)
	if got := names(c.Clear(map[string]bool{"org/b": true})); len(got) != 1 || got[0] != "org/a" {
		t.Fatalf("cleared %v", got)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("a file that is not a model was removed")
	}
	if got := names(c.List(nil)); len(got) != 1 || got[0] != "org/b" {
		t.Fatalf("left %v", got)
	}
}

func TestDirHonorsTheOverride(t *testing.T) {
	want := filepath.Join(t.TempDir(), "cache")
	t.Setenv("ZONN_MODEL_CACHE", want)
	if got, err := Dir(); err != nil || got != want {
		t.Fatalf("Dir() = %q, %v", got, err)
	}
	t.Setenv("ZONN_MODEL_CACHE", "")
	home := t.TempDir()
	t.Setenv("ZONN_HOME", home)
	if got, _ := Dir(); got != filepath.Join(home, "models") {
		t.Fatalf("default Dir() = %q", got)
	}
}
