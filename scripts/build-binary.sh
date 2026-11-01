#!/usr/bin/env bash
set -euo pipefail

arch=${1:?Usage: scripts/build-binary.sh amd64|arm64 [output]}
output=${2:-maxifeed-linux-$arch}
version_package=miniflux.app/v2/internal/version

cd "$(dirname "$0")/.."

echo "Building $output for linux/$arch..."
CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X $version_package.Commit=$(git rev-parse --short=8 HEAD) -X $version_package.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o "$output" .
