# Zonn Node

The desktop app contributors install to share part of their machine with
Zonn. Wails v2: a Go daemon plus a React, TypeScript, and Tailwind window,
compiled to one native binary per operating system. Open source under the
Apache License 2.0 (see `LICENSE` and `NOTICE`; the Zonn name and logo are
trademarks and not licensed).

Zonn Node connects to the Zonn backend (a separate, private repository) and,
when a Zone uses the machine, runs the backend's inference runner, which the
installers bundle.

## Design

The engine and the window are separate on purpose. All the real work lives in a
headless daemon (`internal/daemon`) that detects hardware, talks to the
Coordinator, and serves live data on `127.0.0.1:9090`. The Wails window is a thin
viewer that reads that same local API, so the frontend runs unchanged in the
desktop webview or in a plain browser during development. The daemon also runs on
its own with no window, which is the roadmap's headless fallback.

## Layout

```
cmd/noded/            headless daemon runner (no window, fully testable)
internal/config/      ~/.zonn/config.json, plus the node credential in credentials.json (0600)
internal/resources/   hardware detection (gopsutil) and GPU (nvidia-smi, system_profiler)
internal/client/      gRPC client to the backend
internal/nodeproto/   code generated from proto/node.proto
proto/node.proto      the node protocol, an identical copy of the backend's
scripts/              fetch-runner.sh, check-proto.sh, gen-proto.sh
internal/daemon/      the engine: connect, register, heartbeat, reconnect, state
internal/localapi/    local dashboard API on 127.0.0.1:9090, this app's origins only
internal/version/     build version, set by the release workflow
main.go, app.go       Wails desktop wrapper (build tag: desktop)
wails.json            Wails project config
frontend/             React, TypeScript, Tailwind dashboard
build/                icons and installer scripts (Windows, Ubuntu, macOS)
```

## Run the daemon (headless, works anywhere)

```
go run ./cmd/noded -coordinator localhost:8080 -enroll ze_...   # code from Zonn Console, Nodes page
```

To serve Zones from source, point it at the runner in a backend checkout:
`ZONN_WORKER_SCRIPT=../backend/runner/inference_worker.py` and
`ZONN_PYTHON` at a Python with `../backend/runner/requirements.txt` and
PyTorch installed. `ZONN_HOME` gives a node its own settings folder and
`ZONN_FINGERPRINT` its own identity, to run several on one machine.

Settings live in `~/.zonn/`. Versions before the rename used `~/.colony/`: on
first start Zonn Node copies that folder to `~/.zonn/`, checks the copy, and
removes the old one, keeping the credential so the machine stays enrolled (if
the copy fails it keeps using `~/.colony/`). The `COLONY_*` variable names
still work when the `ZONN_*` name is unset.

`-enroll` is only needed once; the credential is saved for later runs.

Then open `http://localhost:9090/api/state` to see live node state, or point the
frontend at it with `cd frontend && npm install && npm run dev`.

Local API:

```
GET  /health        liveness
GET  /api/state     live node state, specs, utilization, activity log
GET  /api/config    current settings (never the credential)
POST /api/config    update allocation and settings, applied immediately
POST /api/enroll    {"code": "ze_..."} enroll with a code from Zonn Console
```

The local API listens on loopback only, requires a loopback Host header (which
defeats DNS rebinding), and answers browsers only from the app's own webview
(`wails://wails`, `http://wails.localhost`) or a local development server, so a
web page on another site cannot change the node's settings.

## Build the desktop app

The desktop binary needs a webview (webkit2gtk on Linux, WebView2 on Windows, the
system WebView on macOS) and the Wails CLI. It is built with the `desktop` tag,
which is why `go build ./...` on its own skips the window and only builds the
daemon.

```
go install github.com/wailsapp/wails/v2/cmd/wails@latest
# Linux also needs the webkit2gtk and gtk3 development packages, for example on
# Ubuntu: sudo apt install libwebkit2gtk-4.1-dev libgtk-3-dev
wails build -tags "desktop,webkit2_41" -skipbindings   # produces build/bin/zonn-node
```

The `webkit2_41` tag points cgo at webkit2gtk-4.1 (what current Ubuntu ships)
rather than the old 4.0; it matches nothing on macOS or Windows, so the same
command works there. Bindings are skipped because the frontend reads the local
daemon on :9090 and imports no Wails bindings, and every root .go file sits
behind the desktop tag, which would otherwise fail binding generation. Use
`wails dev -tags "desktop,webkit2_41"` to run it live.

Installers bundle a native `inference-worker` containing Python, numpy, CPU
PyTorch, and Transformers for both mock and real Playground runs, so contributors
install no language runtime or dependencies. Models download on first use and are
then read from the local Hugging Face cache. A source or custom deployment can put that
script in `~/.zonn/` or set `ZONN_WORKER_SCRIPT`.

Install candidates are a Windows Setup `.exe`, an Ubuntu `.deb`, and a macOS
`.dmg`. The build workflow uploads them for clean-machine verification and never
publishes a GitHub release automatically.

## Status

Hardware detection (including GPU), enrollment, registration, heartbeat
streaming, reconnect with backoff, the local dashboard API, and the desktop
window, all verified live against the backend.

## Running in the background

- **Closing the window keeps contributing.** The app stays running; reopen it
  from the tray icon (Windows, and Linux desktops with a StatusNotifier tray,
  such as Ubuntu's GNOME, KDE, and most others) or by launching Zonn Node again,
  which opens the running app instead of starting a second one. The tray menu
  shows what the node is doing and has Open, Pause or Resume, and Quit. On
  macOS the Zonn Node menu in the menu bar (click the Dock icon) has the same
  actions; a menu bar icon of its own waits for Wails v3.
- **Start when you sign in** (Settings) adds a Windows Run entry under the
  current user, a macOS LaunchAgent (`~/Library/LaunchAgents/com.zonnhq.node.plist`),
  or an XDG autostart entry (`~/.config/autostart/zonn-node.desktop`), starting
  the app hidden (`--hidden`). Turning it off removes the entry; the Windows
  uninstaller removes it too, and the Linux entry is skipped once the app is
  uninstalled.
- **Only when idle** (Settings) contributes only after nobody has used the
  keyboard or mouse for a chosen number of minutes, and pauses within about 5
  seconds of the owner coming back; a Zone using the machine rebuilds its work
  on its other machines. The idle time comes from `GetLastInputInfo` on
  Windows, `ioreg` on macOS, and on Linux GNOME's idle monitor or the
  freedesktop screensaver service over D-Bus, or `xprintidle`. Where none is
  available the node does not contribute and says why.

## Updates

Zonn Node checks the backend for a newer version when it starts and every 6
hours (`internal/updater`, over the node's own gRPC connection). A newer
version shows in the window and the tray with **Download and install**: the
installer downloads into `~/.zonn/updates` (resuming an interrupted download),
is checked against the SHA-256 the backend published, and only then opens.
A file that does not match is deleted. Release builds stamp their version and
variant (`-X .../internal/version.Version=v1.2.3 -X .../internal/version.Variant=nvidia`)
so the backend offers the matching installer.

`scripts/release-manifest.py VERSION FOLDER` writes `zonn-node-release.json`
and `SHA256SUMS` for a folder of installers; the release workflow's last job
runs it.

## Installers

Each installer bundles the self-contained inference runner, built by the
backend. Two variants:

| Variant | Windows | Ubuntu | macOS |
|---|---|---|---|
| Standard (CPU, and Apple GPUs on macOS) | `Zonn-Node-Setup-Windows-x64.exe` | `zonn-node_VERSION_amd64.deb` | `Zonn-Node-VERSION-macOS-*.dmg` |
| NVIDIA (CUDA 12.9 runtime, needs a recent NVIDIA driver) | `Zonn-Node-Setup-Windows-x64-NVIDIA.exe` | `zonn-node-nvidia_VERSION_amd64.deb` | not needed |

The NVIDIA variant is several gigabytes larger. Either one replaces the other
when installed.

Locally, with the backend checked out next to this repository:

```
(cd ../backend && make runner-cpu)          # or runner-nvidia
scripts/fetch-runner.sh cpu                 # copies it into build/worker/
wails build -tags "desktop,webkit2_41" -skipbindings
build/linux/build-deb.sh 0.2.0 cpu          # or nvidia
```

The release workflow does the same on every platform, fetching the runner
bundles from a backend release (`BACKEND_RELEASE_TOKEN` secret).

## Protocol

`proto/node.proto` must match the backend's `api/proto/node.proto`:
`scripts/check-proto.sh` compares them, and `scripts/gen-proto.sh` regenerates
`internal/nodeproto` after a change.
