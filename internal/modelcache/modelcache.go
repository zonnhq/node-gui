// Package modelcache manages the model weights Zonn Node's stage workers
// download. Workers keep them in a folder of Zonn Node's own (in the Hugging
// Face cache layout, models--org--name), never in the Hugging Face cache the
// contributor may use for other work, so removing a model here can never
// touch anything else. The folder is kept under a size cap by removing the
// models used least recently, never one a running stage needs.
package modelcache

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zonnhq/node-gui/internal/config"
)

// usageFile records when each model was last used, inside the cache folder.
const usageFile = "zonn-usage.json"

// Dir returns the model cache folder: ZONN_MODEL_CACHE, or models in the
// Zonn Node folder.
func Dir() (string, error) {
	if dir := config.Env("ZONN_MODEL_CACHE"); dir != "" {
		return dir, os.MkdirAll(dir, 0o755)
	}
	base, err := config.Dir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "models")
	return dir, os.MkdirAll(dir, 0o755)
}

// Model is one model in the cache.
type Model struct {
	Name     string    `json:"name"` // org/name
	Bytes    int64     `json:"bytes"`
	LastUsed time.Time `json:"last_used"`
	InUse    bool      `json:"in_use"`
}

// Cache is a model cache folder.
type Cache struct {
	dir string
	mu  sync.Mutex // serializes scans, removals, and usage writes
}

// New opens the cache in dir.
func New(dir string) *Cache { return &Cache{dir: dir} }

// Dir is the cache folder.
func (c *Cache) Dir() string { return c.dir }

// folder is a model's folder name in the Hugging Face layout.
func folder(name string) string { return "models--" + strings.ReplaceAll(name, "/", "--") }

// Touch records that a model is being used now.
func (c *Cache) Touch(name string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	usage := c.readUsage()
	usage[name] = now.UTC()
	c.writeUsage(usage)
}

// List returns the models in the cache, least recently used first, marking
// those in inUse.
func (c *Cache) List(inUse map[string]bool) []Model {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.listLocked(inUse)
}

func (c *Cache) listLocked(inUse map[string]bool) []Model {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil
	}
	usage := c.readUsage()
	var out []Model
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), "models--")
		if !e.IsDir() || !ok {
			continue
		}
		name := strings.Replace(rest, "--", "/", 1)
		path := filepath.Join(c.dir, e.Name())
		m := Model{Name: name, Bytes: size(path), InUse: inUse[name], LastUsed: usage[name]}
		if m.LastUsed.IsZero() {
			if info, err := e.Info(); err == nil {
				m.LastUsed = info.ModTime().UTC()
			}
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LastUsed.Before(out[j].LastUsed) })
	return out
}

// size adds up a model folder's files. Snapshots are symbolic links to the
// blobs, except where the system does not allow links and the files are
// copied, so links are skipped and copies counted.
func size(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// Fit removes the least recently used models not in inUse until the cache
// holds at most capBytes, and returns the models removed. A cap of zero or
// less means no limit.
func (c *Cache) Fit(capBytes int64, inUse map[string]bool) []Model {
	if capBytes <= 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	models := c.listLocked(inUse)
	var total int64
	for _, m := range models {
		total += m.Bytes
	}
	var removed []Model
	for _, m := range models {
		if total <= capBytes {
			break
		}
		if m.InUse {
			continue
		}
		if c.removeLocked(m.Name) == nil {
			total -= m.Bytes
			removed = append(removed, m)
		}
	}
	return removed
}

// Clear removes every model not in inUse and returns them.
func (c *Cache) Clear(inUse map[string]bool) []Model {
	c.mu.Lock()
	defer c.mu.Unlock()
	var removed []Model
	for _, m := range c.listLocked(inUse) {
		if !m.InUse && c.removeLocked(m.Name) == nil {
			removed = append(removed, m)
		}
	}
	return removed
}

func (c *Cache) removeLocked(name string) error {
	if err := os.RemoveAll(filepath.Join(c.dir, folder(name))); err != nil {
		return err
	}
	_ = os.RemoveAll(filepath.Join(c.dir, ".locks", folder(name)))
	usage := c.readUsage()
	delete(usage, name)
	c.writeUsage(usage)
	return nil
}

func (c *Cache) readUsage() map[string]time.Time {
	usage := map[string]time.Time{}
	if data, err := os.ReadFile(filepath.Join(c.dir, usageFile)); err == nil {
		_ = json.Unmarshal(data, &usage)
	}
	return usage
}

func (c *Cache) writeUsage(usage map[string]time.Time) {
	data, err := json.Marshal(usage)
	if err != nil {
		return
	}
	tmp := filepath.Join(c.dir, usageFile+".tmp")
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, filepath.Join(c.dir, usageFile))
	}
}
