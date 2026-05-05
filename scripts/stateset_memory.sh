#!/usr/bin/env bash
# stateset_memory.sh — measure peak RSS for native stateset vs float-gauge querying.
#
# Each scenario runs in its own process so VmHWM starts clean.
# Mirrors what `systemd-run --wait` would report for MemoryPeak, but
# uses /proc/{pid}/status polling since systemd-run is not available here.
#
# Usage:
#   bash scripts/stateset_memory.sh
#
# Output includes both the test's own log lines (HeapInuse, VmRSS) and
# the externally observed peak VmRSS from /proc polling.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TEST_BIN=$(mktemp /tmp/tsdb.test.XXXXXX)

echo "==> Building test binary..."
cd "$REPO_ROOT"
go test -c -o "$TEST_BIN" ./tsdb/
trap 'rm -f "$TEST_BIN"' EXIT

# poll_peak_rss <pid> — background function that polls /proc/<pid>/status
# and writes the peak VmRSS seen to stdout when the process exits.
poll_peak_rss() {
    local pid=$1
    local peak=0
    while kill -0 "$pid" 2>/dev/null; do
        rss=$(grep '^VmRSS:' "/proc/$pid/status" 2>/dev/null | awk '{print $2}')
        if [[ -n "$rss" && "$rss" -gt "$peak" ]]; then
            peak=$rss
        fi
        sleep 0.05
    done
    echo "$peak"
}

run_scenario() {
    local name=$1
    local filter=$2

    echo ""
    echo "=== $name ==="

    # Start test in background, capture its output.
    "$TEST_BIN" -test.run="^TestStatesetMemory/${filter}$" -test.v \
        -test.timeout=600s 2>&1 &
    local pid=$!

    # Monitor peak RSS in a subshell.
    peak_rss=$(poll_peak_rss "$pid")

    wait "$pid"
    echo "--- external observer: peak VmRSS = ${peak_rss} KiB ($(echo "scale=1; $peak_rss/1024" | bc) MiB)"
}

run_scenario "native_stateset (1 series per pod × ${memTestPods:-2000} pods)" "native_stateset"
run_scenario "float_gauges   (5 series per pod × ${memTestPods:-2000} pods)" "float_gauges"

echo ""
echo "==> Done. Compare 'after append+GC' HeapInuse lines for storage cost."
echo "    Compare 'peak VmRSS' lines for total process RSS including mmap'd WAL."
