#!/usr/bin/env bash
# Puts the self-contained inference runner into build/worker/ for the
# installers. The runner's source is in the private backend repository, which
# builds the bundles; this repository only packages them.
#
#   scripts/fetch-runner.sh cpu|nvidia
#       Copy a bundle built locally with the backend's `make runner-cpu` or
#       `make runner-nvidia` (from $ZONN_BACKEND_DIR, default ../backend).
#
#   scripts/fetch-runner.sh cpu|nvidia --release TAG
#       Download it from the backend's GitHub release TAG, as CI does. Needs
#       GH_TOKEN with read access to $BACKEND_REPO, and RUNNER_LABEL (for
#       example linux-x64, windows-x64, macos-apple-silicon).
set -euo pipefail

variant="${1:?usage: fetch-runner.sh cpu|nvidia [--release TAG]}"
root="$(cd "$(dirname "$0")/.." && pwd)"
dest="$root/build/worker"
rm -rf "$dest"
mkdir -p "$dest"

if [ "${2:-}" = "--release" ]; then
  tag="${3:?missing release tag}"
  label="${RUNNER_LABEL:?set RUNNER_LABEL}"
  repo="${BACKEND_REPO:?set BACKEND_REPO, for example zonnhq/backend}"
  name="inference-worker-${label}-${variant}.zip"
  work="$(mktemp -d)"
  gh release download "$tag" --repo "$repo" --dir "$work" --pattern "$name" --pattern "$name.part*"
  if [ ! -f "$work/$name" ]; then
    cat "$work/$name".part* > "$work/$name"
  fi
  (cd "$dest" && unzip -q "$work/$name")
else
  backend="${ZONN_BACKEND_DIR:-$root/../backend}"
  src="$backend/runner/dist/$variant"
  if [ ! -d "$src" ]; then
    echo "no $variant runner in $src; build it with 'make runner-$variant' in the backend" >&2
    exit 1
  fi
  cp -R "$src"/. "$dest"/
fi

# A cpu bundle is one file; an nvidia bundle is a folder of the same name.
ls -la "$dest"
