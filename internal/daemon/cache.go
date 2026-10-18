package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// cacheCheckInterval is how often the model cache is rescanned and held to
// its cap, besides whenever a stage starts or ends or a download finishes.
const cacheCheckInterval = 5 * time.Minute

const bytesPerGB = 1 << 30

// modelsInUse returns the models running stage workers need, which are
// never removed from the cache.
func (d *Daemon) modelsInUse() map[string]bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	inUse := map[string]bool{}
	for _, w := range d.workers {
		if w.model != "" {
			inUse[w.model] = true
		}
	}
	return inUse
}

// fitCache removes the least recently used models not in use until the
// cache is within its cap, then rescans it for the window.
func (d *Daemon) fitCache() {
	if d.cache == nil {
		return
	}
	capGB := d.Config().ModelCacheGB
	removed := d.cache.Fit(int64(capGB)*bytesPerGB, d.modelsInUse())
	for _, m := range removed {
		d.log("INFO", fmt.Sprintf("Removed %s (%s) from the model cache to stay under its %d GB limit", m.Name, megabytes(m.Bytes), capGB))
	}
	d.refreshCache()
}

// ClearCache removes every model no running stage needs and returns how many
// it removed.
func (d *Daemon) ClearCache() int {
	if d.cache == nil {
		return 0
	}
	removed := d.cache.Clear(d.modelsInUse())
	if len(removed) > 0 {
		names := make([]string, len(removed))
		var freed int64
		for i, m := range removed {
			names[i] = m.Name
			freed += m.Bytes
		}
		d.log("INFO", fmt.Sprintf("Cleared the model cache: removed %s, freeing %s", strings.Join(names, ", "), megabytes(freed)))
	} else {
		d.log("INFO", "Cleared the model cache: nothing to remove that is not in use")
	}
	d.refreshCache()
	return len(removed)
}

// refreshCache rescans the cache folder for the snapshot.
func (d *Daemon) refreshCache() {
	if d.cache == nil {
		return
	}
	models := d.cache.List(d.modelsInUse())
	state := CacheState{Dir: d.cache.Dir(), Models: models}
	for _, m := range models {
		state.Bytes += m.Bytes
	}
	d.mu.Lock()
	d.cacheState = state
	d.mu.Unlock()
}

// watchCache holds the cache to its cap on a clock until ctx ends, which also
// catches a cap lowered while the node was not running.
func (d *Daemon) watchCache(ctx context.Context) {
	defer d.recoverCrash("watchCache")
	d.fitCache()
	ticker := time.NewTicker(cacheCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.fitCache()
		}
	}
}

func megabytes(n int64) string {
	if n >= 10*bytesPerGB {
		return fmt.Sprintf("%.0f GB", float64(n)/bytesPerGB)
	}
	if n >= bytesPerGB {
		return fmt.Sprintf("%.1f GB", float64(n)/bytesPerGB)
	}
	return fmt.Sprintf("%d MB", (n+(1<<19))>>20)
}
