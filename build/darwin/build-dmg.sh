#!/usr/bin/env bash
# Builds the macOS disk image from build/bin/zonn-node.app and the runner in
# build/worker/ (scripts/fetch-runner.sh cpu; macOS PyTorch includes MPS for
# Apple GPUs, so there is no separate NVIDIA variant).
#
#   build/darwin/build-dmg.sh VERSION [apple-silicon|intel]
set -euo pipefail

version="${1:-0.2.0-dev}"
architecture="${2:-apple-silicon}"
root="$(cd "$(dirname "$0")/../.." && pwd)"
app="$root/build/bin/zonn-node.app"
output_dir="$root/build/installer"
staging="$root/build/dmg-staging"

mkdir -p "$app/Contents/Resources" "$output_dir"
install -m 0755 "$root/build/worker/inference-worker" "$app/Contents/Resources/inference-worker"

rm -rf "$staging"
mkdir -p "$staging"
cp -R "$app" "$staging/Zonn Node.app"
ln -s /Applications "$staging/Applications"

hdiutil create -volname "Zonn Node" -srcfolder "$staging" -ov -format UDZO \
  "$output_dir/Zonn-Node-${version}-macOS-${architecture}.dmg"
