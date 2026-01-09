#!/bin/bash
# Copyright The Prometheus Authors
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Semantic Conventions Translation Browser Demo
#
# This script:
# 1. Populates a TSDB with sample metrics using different OTLP naming conventions
# 2. Launches Prometheus to serve the data via browser UI
# 3. Opens the browser so you can query with __semconv_url__
#
# Usage: ./demo.sh [port]
# Example: ./demo.sh 9091  # Use port 9091 instead of default 9090

set -e

# Port to use (default 9090, or first argument)
PORT="${1:-9090}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
DATA_DIR="$SCRIPT_DIR/demo-data"

echo -e "\n${BOLD}${CYAN}=== Semconv Translation Browser Demo ===${NC}\n"

# Clean up any existing data
if [ -d "$DATA_DIR" ]; then
    echo -e "${YELLOW}Removing existing demo data...${NC}"
    rm -rf "$DATA_DIR"
fi

# Build Prometheus if needed
PROMETHEUS_BIN="$REPO_ROOT/prometheus"
if [ ! -f "$PROMETHEUS_BIN" ]; then
    echo -e "${YELLOW}Building Prometheus...${NC}"
    cd "$REPO_ROOT"
    make build
fi

# Populate the TSDB
echo -e "${GREEN}Populating TSDB with sample data...${NC}\n"
cd "$REPO_ROOT"
go run ./documentation/examples/semconv-translation --data-dir="$DATA_DIR" --populate-only

# Create a minimal config file in the data directory (cleaned up with data)
CONFIG_FILE="$DATA_DIR/prometheus-demo.yml"
cat > "$CONFIG_FILE" << 'EOF'
# Minimal Prometheus config for semconv demo
global:
  scrape_interval: 15s
EOF

# Function to open browser (cross-platform)
open_browser() {
    local url=$1
    if command -v xdg-open &> /dev/null; then
        xdg-open "$url" &> /dev/null &
    elif command -v open &> /dev/null; then
        open "$url" &
    elif command -v start &> /dev/null; then
        start "$url" &
    else
        echo -e "${YELLOW}Please open your browser to: $url${NC}"
    fi
}

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    # Note: We keep demo-data (including config file) for inspection. Delete manually if needed.
    echo -e "${GREEN}Demo data preserved at: $DATA_DIR${NC}"
    echo -e "${GREEN}Run 'rm -rf $DATA_DIR' to clean up.${NC}"
}
trap cleanup EXIT

echo -e "${BOLD}${GREEN}Starting Prometheus server...${NC}"
echo -e "  Web UI: ${CYAN}http://localhost:${PORT}${NC}\n"

echo -e "${BOLD}Try these queries in the browser:${NC}\n"
echo -e "  ${YELLOW}The Problem - Queries break after migration:${NC}"
echo -e "    ${CYAN}test_bytes_total${NC}                           - Only old data (before migration)"
echo -e "    ${CYAN}test${NC}                                       - Only new data (after migration)\n"
echo -e "  ${GREEN}The Solution - Seamless query continuity:${NC}"
echo -e "    ${CYAN}test{__semconv_url__=\"registry/1.1.0\"}${NC}      - ALL data, unified naming!"
echo -e "    ${CYAN}sum(test{__semconv_url__=\"registry/1.1.0\"})${NC} - Aggregation works"
echo -e "    ${CYAN}rate(test{__semconv_url__=\"registry/1.1.0\"}[5m])${NC} - Rate works too!\n"

echo -e "${YELLOW}Press Ctrl+C to stop the demo.${NC}\n"

# Give user a moment to read, then open browser
sleep 2
open_browser "http://localhost:${PORT}/graph?g0.expr=test%7B__semconv_url__%3D%22registry%2F1.1.0%22%7D&g0.tab=0&g0.range_input=2h"

# Start Prometheus (this blocks until Ctrl+C)
# Note: --enable-feature=semconv-versioned-read is required for __semconv_url__ queries
"$PROMETHEUS_BIN" \
    --storage.tsdb.path="$DATA_DIR" \
    --config.file="$CONFIG_FILE" \
    --web.listen-address=":${PORT}" \
    --enable-feature=semconv-versioned-read \
    --log.level=warn
