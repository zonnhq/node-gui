#!/usr/bin/env bash
# The node protocol's canonical copy is in the backend repository
# (api/proto/node.proto). This repository keeps an identical copy; this check
# fails if they drift. Pass the backend's path (default ../backend).
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
backend="${1:-$root/../backend}"
if [ ! -f "$backend/api/proto/node.proto" ]; then
  echo "backend not found at $backend; skipped"
  exit 0
fi
if diff -u "$backend/api/proto/node.proto" "$root/proto/node.proto"; then
  echo "node.proto matches the backend's"
else
  echo "proto/node.proto differs from the backend's api/proto/node.proto" >&2
  exit 1
fi
