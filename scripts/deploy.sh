#!/usr/bin/env bash
set -euo pipefail

host=${1:?Usage: scripts/deploy.sh ssh-host [service]}
service=${2:-miniflux}

case "$(ssh "$host" uname -m)" in
    x86_64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *)
        echo "Unsupported server architecture." >&2
        exit 1
        ;;
esac

build_dir=$(mktemp -d)
trap 'rm -rf "$build_dir"' EXIT

"$(dirname "$0")/build-binary.sh" "$arch" "$build_dir/maxifeed"

echo "Uploading to $host..."
scp -q "$build_dir/maxifeed" "$host:/tmp/maxifeed.new"

ssh "$host" 'cat > /tmp/maxifeed-install.sh' <<'REMOTE'
set -eu

service=$1
sudo=$([ "$(id -u)" = 0 ] || echo sudo)
binary=$(systemctl show "$service" --property=ExecStart --value | sed -E 's/.*path=([^ ;]+).*/\1/')

if [ ! -x "$binary" ]; then
    echo "Could not find the binary of the $service service." >&2
    exit 1
fi

$sudo cp -p "$binary" "$binary.previous"
$sudo install -m 755 /tmp/maxifeed.new "$binary"
rm -f /tmp/maxifeed.new

if $sudo systemctl restart "$service" && sleep 5 && systemctl is-active --quiet "$service"; then
    echo "Deployed. The previous version is kept at $binary.previous."
else
    echo "The new version did not start, so the previous one is back in place." >&2
    $sudo install -m 755 "$binary.previous" "$binary"
    $sudo systemctl restart "$service"
    exit 1
fi
REMOTE

ssh -t "$host" sh /tmp/maxifeed-install.sh "$service"
