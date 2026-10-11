package autostart

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFiles(t *testing.T) {
	entry := desktopEntry("/usr/bin/zonn-node")
	for _, want := range []string{`Exec="/usr/bin/zonn-node" --hidden`, "TryExec=/usr/bin/zonn-node", "Type=Application"} {
		if !strings.Contains(entry, want) {
			t.Fatalf("desktop entry lacks %q:\n%s", want, entry)
		}
	}
	plist := launchAgent("com.zonnhq.node", "/Applications/Zonn Node.app/Contents/MacOS/zonn-node")
	for _, want := range []string{"<string>/Applications/Zonn Node.app/Contents/MacOS/zonn-node</string>", "<string>--hidden</string>", "<key>RunAtLoad</key>"} {
		if !strings.Contains(plist, want) {
			t.Fatalf("launch agent lacks %q", want)
		}
	}
	if got := runValue(`C:\Program Files\Zonn Node\zonn-node.exe`); got != `"C:\Program Files\Zonn Node\zonn-node.exe" --hidden` {
		t.Fatalf("run value %q", got)
	}
}

func TestLinuxEntryRoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if Enabled() {
		t.Fatal("enabled before anything was written")
	}
	if err := Set(true, "/opt/zonn/zonn-node"); err != nil || !Enabled() {
		t.Fatalf("enable: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "autostart", "zonn-node.desktop"))
	if !strings.Contains(string(data), "/opt/zonn/zonn-node") {
		t.Fatalf("entry: %s", data)
	}
	if err := Set(false, ""); err != nil || Enabled() {
		t.Fatalf("disable: %v", err)
	}
	if err := Set(false, ""); err != nil {
		t.Fatalf("disabling twice: %v", err)
	}
}
