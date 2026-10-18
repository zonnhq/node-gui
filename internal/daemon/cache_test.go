package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zonnhq/node-gui/internal/config"
	"github.com/zonnhq/node-gui/internal/worker"
)

// cachedModel writes a fake model of n MB into the node's model cache, as a
// sparse file so it takes no real disk.
func cachedModel(t *testing.T, d *Daemon, name string, mb int, used time.Time) {
	t.Helper()
	dir := filepath.Join(d.cache.Dir(), "models--"+strings.ReplaceAll(name, "/", "--"), "blobs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "weights"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(int64(mb) << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()
	d.cache.Touch(name, used)
}

func TestTheCacheCapRemovesOldModelsButNeverOneARunningStageNeeds(t *testing.T) {
	t.Setenv("ZONN_HOME", t.TempDir())
	t.Setenv("ZONN_MODEL_CACHE", "")
	cfg := config.Default()
	cfg.Configured = true
	cfg.ModelCacheGB = 0
	d := New(cfg)
	now := time.Now()
	cachedModel(t, d, "org/oldest", 600, now.Add(-3*time.Hour))
	cachedModel(t, d, "org/older", 600, now.Add(-2*time.Hour))
	cachedModel(t, d, "org/active", 600, now.Add(-4*time.Hour)) // least recent, but in use
	d.workers["dep:stage0"] = &runningWorker{cancel: func() {}, model: "org/active",
		download: &worker.Download{Model: "org/active", Done: 1, Total: 2}}

	// No limit: nothing goes.
	d.fitCache()
	if got := len(d.Snapshot().ModelCache.Models); got != 3 {
		t.Fatalf("%d models with no limit", got)
	}
	// Lowering the cap applies it at once. 1.8 GB against 1 GB: the oldest
	// idle model goes, then the next, and the one in use stays.
	cfg = d.Config()
	cfg.ModelCacheGB = 1
	if err := d.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	st := d.Snapshot()
	if len(st.ModelCache.Models) != 1 || st.ModelCache.Models[0].Name != "org/active" || !st.ModelCache.Models[0].InUse ||
		st.ModelCache.CapGB != 1 || st.ModelCache.Bytes != 600<<20 {
		t.Fatalf("cache after the cap: %+v", st.ModelCache)
	}
	d.workers["dep:stage1"] = &runningWorker{cancel: func() {}, model: "org/active",
		download: &worker.Download{Model: "org/active", Done: 1, Total: 2}}
	st = d.Snapshot()
	delete(d.workers, "dep:stage1")
	if len(st.Downloads) != 1 || st.Downloads[0].Model != "org/active" {
		t.Fatalf("downloads: %+v", st.Downloads)
	}
	removals := 0
	for _, e := range st.Events {
		if strings.HasPrefix(e.Message, "Removed org/old") {
			removals++
		}
	}
	if removals != 2 {
		t.Fatalf("logged %d removals", removals)
	}

	// Clear cache keeps the model in use, and removes it once its stage ends.
	cachedModel(t, d, "org/other", 1, now)
	if n := d.ClearCache(); n != 1 {
		t.Fatalf("cleared %d", n)
	}
	delete(d.workers, "dep:stage0")
	if n := d.ClearCache(); n != 1 || len(d.Snapshot().ModelCache.Models) != 0 {
		t.Fatalf("cleared %d, left %+v", n, d.Snapshot().ModelCache.Models)
	}
}
