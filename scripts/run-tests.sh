#!/usr/bin/env bash
# Runs the ECK-Tunnel tests on a Linux server (Ubuntu 22.04+), as root, in the
# unpacked source folder.
#
#   bash scripts/run-tests.sh          quick: a few minutes — the parts ECK-Tunnel
#                                      changed plus the transport round trips
#   bash scripts/run-tests.sh --full   everything, including soak and chaos tests
#                                      (much longer)
#
# Installs Go into /usr/local/go-eck if missing. Writes test-report.txt.
set -uo pipefail
mode=quick
[[ "${1:-}" == "--full" ]] && mode=full

GO_VERSION="$(sed -n 's/^go //p' go.mod)"
GO_DIR=/usr/local/go-eck
if [[ ! -x "$GO_DIR/bin/go" ]]; then
  arch=amd64; [[ "$(uname -m)" == "aarch64" ]] && arch=arm64
  tgz="go${GO_VERSION}.linux-${arch}.tar.gz"
  for url in "https://go.dev/dl/${tgz}" "https://mirrors.aliyun.com/golang/${tgz}"; do
    echo "downloading ${url}"
    curl -fL --connect-timeout 10 -o "/tmp/${tgz}" "$url" && break
  done
  rm -rf "$GO_DIR" && mkdir -p "$GO_DIR" && tar -xzf "/tmp/${tgz}" -C "$GO_DIR" --strip-components=1 || { echo "Go install failed"; exit 1; }
fi
export PATH="$GO_DIR/bin:$PATH" CGO_ENABLED=0
export GOPROXY="${GOPROXY:-https://proxy.golang.org,https://goproxy.cn,direct}"
go version
: > test-report.txt
start=$(date +%s)

if [[ "$mode" == quick ]]; then
  # What ECK-Tunnel added or changed, in full.
  changed=(./internal/systools/ ./internal/optimize/ ./internal/app/ ./internal/menu/ ./internal/manage/)
  echo "== go vet (changed packages)"
  go vet "${changed[@]}" 2>&1 | tee -a test-report.txt
  echo "== unit tests (changed packages)"
  go test -short -count=1 -timeout 5m "${changed[@]}" 2>&1 | tee -a test-report.txt
  # One round trip per transport, the Stealth transport the Iran route uses,
  # and the token check — the end-to-end paths a rename could have broken.
  echo "== end-to-end transport round trips"
  go test -short -count=1 -timeout 5m -run 'TestEveryTransportEstablishesAndCarriesData|TestTransportCarriesData|TestWrongTokenIsRejected|Stealth|Noise' \
    ./internal/e2e/ ./internal/utils/network/ 2>&1 | tee -a test-report.txt
else
  echo "== go vet"
  go vet ./... 2>&1 | tee -a test-report.txt
  echo "== go test, every package (this takes a long time)"
  go test -count=1 -timeout 30m ./... 2>&1 | tee -a test-report.txt
fi

echo
echo "== summary ($mode, $(( $(date +%s) - start ))s)"
grep -E '^(FAIL|--- FAIL|panic:)' test-report.txt || echo "ALL PASSED"
echo "full log: $(pwd)/test-report.txt"
