#!/usr/bin/env bash
# Builds a signed ECK-Tunnel release into release/.
#
#   RELEASE_SIGNING_KEY="$(cat /path/to/release-signing.key)" bash scripts/make-release.sh
#
# The result is one folder that serves both ways of distributing it:
#   release/<tag>/*        upload as the assets of GitHub release <tag>
#   release/ (all of it)   copy to any web server to use as the update mirror
#                          (ECK_MIRROR for install.sh, Update → Update mirror)
set -euo pipefail
cd "$(dirname "$0")/.."

[[ -n "${RELEASE_SIGNING_KEY:-}" ]] || { echo "RELEASE_SIGNING_KEY is not set — refusing to build an unsigned release." >&2; exit 1; }
tag="$(grep -oE 'Version = "v[0-9.]+"' internal/app/app.go | grep -oE 'v[0-9.]+')"
[[ -n "$tag" ]] || { echo "could not read the version from internal/app/app.go" >&2; exit 1; }

out=release
rm -rf "$out" && mkdir -p "$out/$tag"
for arch in amd64 arm64; do
  echo "building linux/$arch"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags "-s -w" -o "$out/eck" .
  tar -czf "$out/$tag/eck_linux_$arch.tar.gz" -C "$out" eck
  rm -f "$out/eck"
done
(cd "$out/$tag" && sha256sum eck_linux_*.tar.gz > SHA256SUMS)
go run ./tools/signsums "$out/$tag/SHA256SUMS"
echo "$tag" > VERSION
echo "$tag" > "$out/VERSION"
cp install.sh "$out/install.sh"
echo
echo "release $tag ready in $out/:"
find "$out" -type f | sort
