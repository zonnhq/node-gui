// Package version reports this Zonn Node build's version. Release builds set it
// with -ldflags "-X github.com/zonnhq/node-gui/internal/version.Version=v1.2.3";
// local builds report "dev", which the Coordinator's minimum version gate
// always allows.
package version

// Version is the build's version.
var Version = "dev"
