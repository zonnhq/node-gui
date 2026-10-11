package autostart

import (
	"fmt"
	"html"
	"strings"
)

// desktopEntry is the XDG autostart file. TryExec makes the desktop skip it
// once the app has been uninstalled.
func desktopEntry(exe string) string {
	quoted := `"` + strings.ReplaceAll(exe, `"`, `\"`) + `"`
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Zonn Node
Comment=Contribute this machine's spare compute to your Zones
Exec=%s %s
TryExec=%s
Icon=zonn-node
Terminal=false
X-GNOME-Autostart-enabled=true
`, quoted, HiddenFlag, exe)
}

// launchAgent is the macOS LaunchAgent property list.
func launchAgent(label, exe string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`, html.EscapeString(label), html.EscapeString(exe), HiddenFlag)
}

// runValue is the command line stored in the Windows Run key.
func runValue(exe string) string { return `"` + exe + `" ` + HiddenFlag }
