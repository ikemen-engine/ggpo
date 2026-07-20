#!/usr/bin/env bash
# Packages the WebRTC example server for deployment to a remote Linux host.
#
# Produces dist/webrtcserver-<os>-<arch>.tar.gz containing:
#   webrtcserver        the static server binary (signaling + static assets)
#   public/main.wasm    the game compiled to WebAssembly
#   public/wasm_exec.js the Go wasm JS shim matching this toolchain
#
# Override the target with env vars, e.g. GOARCH_TARGET=arm64 for Graviton/Ampere:
#   ./scripts/package.sh
#   GOARCH_TARGET=arm64 ./scripts/package.sh
set -euo pipefail

GOOS_TARGET="${GOOS_TARGET:-linux}"
GOARCH_TARGET="${GOARCH_TARGET:-amd64}"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/public"

echo "building wasm game -> public/main.wasm"
GOOS=js GOARCH=wasm go build -o "$stage/public/main.wasm" ./example/webrtc

echo "copying wasm_exec.js"
goroot="$(go env GOROOT)"
if [ -f "$goroot/lib/wasm/wasm_exec.js" ]; then
  cp "$goroot/lib/wasm/wasm_exec.js" "$stage/public/"
elif [ -f "$goroot/misc/wasm/wasm_exec.js" ]; then
  cp "$goroot/misc/wasm/wasm_exec.js" "$stage/public/"
else
  echo "wasm_exec.js not found under $goroot" >&2
  exit 1
fi

echo "building webrtcserver for $GOOS_TARGET/$GOARCH_TARGET"
CGO_ENABLED=0 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" \
  go build -trimpath -ldflags="-s -w" -o "$stage/webrtcserver" ./cmd/webrtcserver

mkdir -p dist
out="dist/webrtcserver-$GOOS_TARGET-$GOARCH_TARGET.tar.gz"
tar -C "$stage" -czf "$out" webrtcserver public

echo "wrote $out"
tar -tzf "$out"
