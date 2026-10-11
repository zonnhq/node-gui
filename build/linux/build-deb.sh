#!/usr/bin/env bash
# Builds the Ubuntu and Debian package from build/bin/zonn-node and the runner
# in build/worker/ (scripts/fetch-runner.sh).
#
#   build/linux/build-deb.sh VERSION [cpu|nvidia]
#
# The NVIDIA variant is a separate package, zonn-node-nvidia, that replaces
# the standard one; the two cannot be installed together.
set -euo pipefail

version="${1:-0.2.0-dev}"
variant="${2:-cpu}"
root="$(cd "$(dirname "$0")/../.." && pwd)"
package_root="$root/build/package-linux-$variant"
output_dir="$root/build/installer"

name="zonn-node"
conflicts="zonn-node-nvidia"
description="Contribute CPU, memory, and GPU capacity to a Zonn Zone."
if [ "$variant" = nvidia ]; then
  name="zonn-node-nvidia"
  conflicts="zonn-node"
  description="Contribute CPU, memory, and NVIDIA GPU capacity to a Zonn Zone (CUDA runtime included)."
fi

rm -rf "$package_root"
mkdir -p "$package_root/DEBIAN" "$package_root/usr/bin" "$package_root/usr/lib/zonn" \
  "$package_root/usr/share/applications" "$package_root/usr/share/icons/hicolor/512x512/apps" "$output_dir"

install -m 0755 "$root/build/bin/zonn-node" "$package_root/usr/bin/zonn-node"
if [ -d "$root/build/worker/inference-worker" ]; then
  cp -R "$root/build/worker/inference-worker" "$package_root/usr/lib/zonn/inference-worker"
  chmod 0755 "$package_root/usr/lib/zonn/inference-worker/inference-worker"
else
  install -m 0755 "$root/build/worker/inference-worker" "$package_root/usr/lib/zonn/inference-worker"
fi
install -m 0644 "$root/build/linux/zonn-node.desktop" "$package_root/usr/share/applications/zonn-node.desktop"
install -m 0644 "$root/build/appicon.png" "$package_root/usr/share/icons/hicolor/512x512/apps/zonn-node.png"

cat > "$package_root/DEBIAN/control" <<CONTROL
Package: $name
Version: $version
Section: utils
Priority: optional
Architecture: amd64
Maintainer: Zonn
Depends: libwebkit2gtk-4.1-0, libgtk-3-0
Conflicts: $conflicts
Replaces: $conflicts
Description: Zonn contributor node
 $description
CONTROL

dpkg-deb --root-owner-group --build "$package_root" "$output_dir/${name}_${version}_amd64.deb"
