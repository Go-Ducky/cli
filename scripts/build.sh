#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="$ROOT/dist"
VERSION="${VERSION:-0.1.0}"

rm -rf "$DIST"
mkdir -p "$DIST"

build_one() {
  local os="$1" arch="$2" ext="$3"
  local name="goducky-${os}-${arch}${ext}"
  echo "Building $name"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o "$DIST/$name" \
      "$ROOT/cmd/goducky"
}

build_one windows amd64 ".exe"
build_one windows arm64 ".exe"

build_one darwin amd64 ""
build_one darwin arm64 ""

build_one linux amd64 ""
build_one linux arm64 ""

if [[ "$(uname -s)" == "Darwin" ]]; then
  echo "Creating universal macOS binary with lipo"
  lipo -create \
    "$DIST/goducky-darwin-amd64" \
    "$DIST/goducky-darwin-arm64" \
    -output "$DIST/goducky-darwin-universal"
fi

echo ""
echo "Build complete. Artifacts in $DIST/"
ls -la "$DIST"
