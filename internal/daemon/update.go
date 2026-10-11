package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/zonnhq/node-gui/internal/client"
	"github.com/zonnhq/node-gui/internal/config"
	"github.com/zonnhq/node-gui/internal/updater"
	"github.com/zonnhq/node-gui/internal/version"
)

// UpdateState is what the window and tray show about a newer Zonn Node.
type UpdateState struct {
	Available bool   `json:"available"`
	Required  bool   `json:"required"` // the backend refuses this version
	Version   string `json:"version"`
	Size      int64  `json:"size"`
	// Status is "" (nothing started), "downloading", "opened" (the installer
	// is open for the contributor), or "error".
	Status string `json:"status"`
	Done   int64  `json:"done"` // bytes downloaded
	Error  string `json:"error,omitempty"`
	offer  *client.UpdateOffer
}

// Update checks happen shortly after start and then every few hours.
const (
	updateFirstCheck = 10 * time.Second
	updateEvery      = 6 * time.Hour
)

// watchUpdates checks for a newer Zonn Node until ctx ends.
func (d *Daemon) watchUpdates(ctx context.Context) {
	defer d.recoverCrash("watchUpdates")
	wait := updateFirstCheck
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		d.CheckUpdate(ctx)
		wait = updateEvery
	}
}

// CheckUpdate asks the backend whether a newer Zonn Node is published for
// this platform. It works even when the backend refuses this version, so an
// outdated node can still update itself.
func (d *Daemon) CheckUpdate(ctx context.Context) {
	creds := d.credentials()
	if creds == nil {
		return
	}
	cli, err := client.Dial(d.Config().CoordinatorURL, creds)
	if err != nil {
		return
	}
	defer cli.Close()
	callCtx, cancel := context.WithTimeout(ctx, registerTTL)
	defer cancel()
	p := updater.Current()
	offer, err := cli.CheckUpdate(callCtx, creds.NodeID, p.OS, p.Arch, p.Variant, p.Format, version.Version)
	if err != nil {
		return // an older backend without updates, or unreachable: try later
	}
	d.mu.Lock()
	was := d.update
	if offer.Available {
		if was.Version != offer.Version {
			d.update = UpdateState{}
		}
		d.update.Available, d.update.Required, d.update.Version, d.update.Size = true, offer.Required, offer.Version, offer.Size
		d.update.offer = &offer
	} else if was.Status != "downloading" {
		d.update = UpdateState{Required: offer.Required}
	}
	d.mu.Unlock()
	if offer.Available && was.Version != offer.Version {
		d.log("INFO", "Zonn Node "+offer.Version+" is available")
	}
}

// InstallUpdate downloads the offered installer in the background, checks its
// SHA-256, and opens it. It returns at once; progress shows in the state.
func (d *Daemon) InstallUpdate() error {
	d.mu.Lock()
	offer := d.update.offer
	if offer == nil || !d.update.Available {
		d.mu.Unlock()
		return errors.New("no update is available")
	}
	if d.update.Status == "downloading" {
		d.mu.Unlock()
		return nil
	}
	d.update.Status, d.update.Error, d.update.Done = "downloading", "", 0
	addr := d.cfg.CoordinatorURL
	d.mu.Unlock()
	creds := d.credentials()
	if creds == nil {
		return errors.New("enroll this machine first")
	}
	d.log("INFO", "Downloading Zonn Node "+offer.Version)
	go d.downloadUpdate(addr, creds, *offer)
	return nil
}

func (d *Daemon) downloadUpdate(addr string, creds *config.Credentials, offer client.UpdateOffer) {
	defer d.recoverCrash("downloadUpdate")
	fail := func(err error) {
		d.mu.Lock()
		d.update.Status, d.update.Error = "error", err.Error()
		d.mu.Unlock()
		d.log("WARN", "The update could not be installed: "+err.Error())
	}
	dir, err := config.Dir()
	if err != nil {
		fail(err)
		return
	}
	cli, err := client.Dial(addr, creds)
	if err != nil {
		fail(err)
		return
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	dl := func(ctx context.Context, name string, offset int64, w io.Writer) error {
		return cli.DownloadUpdate(ctx, creds.NodeID, name, offset, w)
	}
	progress := func(done int64) {
		d.mu.Lock()
		d.update.Done = done
		d.mu.Unlock()
	}
	path, err := updater.Fetch(ctx, dl, filepath.Join(dir, "updates"), offer.FileName, offer.Size, offer.SHA256, progress)
	if err != nil {
		fail(err)
		return
	}
	d.log("INFO", fmt.Sprintf("Downloaded Zonn Node %s and checked its SHA-256; opening the installer", offer.Version))
	if err := openInstaller(path); err != nil {
		fail(fmt.Errorf("downloaded to %s but could not open it: %w", path, err))
		return
	}
	d.mu.Lock()
	d.update.Status, d.update.Done = "opened", offer.Size
	d.mu.Unlock()
}

// openInstaller is replaced in tests.
var openInstaller = updater.Open
