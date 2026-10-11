#!/usr/bin/env bash
# Regenerates internal/nodeproto from proto/node.proto (needs protoc,
# protoc-gen-go, and protoc-gen-go-grpc on PATH).
set -euo pipefail
cd "$(dirname "$0")/../proto"
pkg=github.com/zonnhq/node-gui/internal/nodeproto
protoc --go_out=../internal/nodeproto --go_opt=paths=source_relative --go_opt=Mnode.proto=$pkg \
  --go-grpc_out=../internal/nodeproto --go-grpc_opt=paths=source_relative --go-grpc_opt=Mnode.proto=$pkg \
  node.proto
