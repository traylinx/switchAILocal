#!/bin/bash

# install-bridge.sh - macOS Service Installer for switchAILocal Bridge
# ==============================================================================

set -e

# --- Configuration ---
PROJECT_DIR="$(pwd)"
BINARY_PATH="${PROJECT_DIR}/bridge-agent"
PLIST_NAME="com.traylinx.switchailocal.bridge.plist"
PLIST_TEMPLATE="${PROJECT_DIR}/com.traylinx.switchailocal.bridge.plist.template"
LAUNCH_AGENTS_DIR="${HOME}/Library/LaunchAgents"
TARGET_PLIST="${LAUNCH_AGENTS_DIR}/${PLIST_NAME}"
LOG_DIR="${HOME}/Library/Logs"
LOG_PATH="${LOG_DIR}/switchAILocal-bridge.log"

# --- Colors ---
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC}   $1"; }
log_error() { echo -e "${RED}[ERR]${NC}  $1"; }

# --- Build ---
log_info "Building bridge-agent binary..."
go build -o "$BINARY_PATH" ./cmd/bridge-agent
log_success "Binary built at ${BINARY_PATH}"

# --- Install Service ---
log_info "Setting up LaunchAgent..."

# Create Plist from template
sed -e "s|{{BINARY_PATH}}|${BINARY_PATH}|g" \
    -e "s|{{LOG_PATH}}|${LOG_PATH}|g" \
    -e "s|{{WORKING_DIR}}|${PROJECT_DIR}|g" \
    "$PLIST_TEMPLATE" > "$TARGET_PLIST"

log_success "Created service file at ${TARGET_PLIST}"

# --- Load Service ---
log_info "Loading service..."

# Unload if already loaded
launchctl unload "$TARGET_PLIST" 2>/dev/null || true

# Load and start
launchctl load -w "$TARGET_PLIST"

log_success "Bridge service installed and started."
log_info "You can check logs with: tail -f ${LOG_PATH}"
log_info "Status can be checked with: ./ail.sh bridge status"
