#!/bin/bash

# ==============================================================================
# swichAILocal Operations Hub (ail.sh)
# ==============================================================================
# This script acts as the unified entry point for all development and operations
# tasks. It abstracts the complexity of Go and Docker commands.
# ==============================================================================

# --- Configuration ---
BINARY_NAME="switchAILocal"
BRIDGE_BINARY="bridge-agent"
STATE_DIR=".ail"
PID_FILE="${STATE_DIR}/local.pid"
BRIDGE_PID_FILE="${STATE_DIR}/bridge.pid"
LOG_FILE="server.log"
BRIDGE_LOG_FILE="bridge-agent.log"
GO_MIN_VERSION="1.24"

# --- Colors ---
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# --- Utilities ---

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC}   $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERR]${NC}  $1"; }

ensure_state_dir() {
    if [ ! -d "$STATE_DIR" ]; then
        mkdir -p "$STATE_DIR"
    fi
}

check_command() {
    if ! command -v "$1" &> /dev/null; then
        return 1
    fi
    return 0
}

# --- Dependency Management ---

check_go() {
    if ! check_command go; then
        log_error "Go is not installed."
        return 1
    fi
    
    # Simple version check (not robust for all edge cases but good for quick check)
    local version
    version=$(go version | awk '{print $3}' | sed 's/go//')
    # minimal check: verify it's not empty. true semantic version comparison is complex in bash.
    if [[ -z "$version" ]]; then
         log_warn "Could not detect Go version."
    else
         log_success "Go detected: $version"
    fi
    return 0
}

check_docker() {
    if ! check_command docker; then
        log_error "Docker is not installed."
        return 1
    fi
    log_success "Docker detected."
    return 0
}

install_dependencies() {
    log_info "Attempting to install dependencies..."
    
    if [[ "$OSTYPE" != "darwin"* ]]; then
        log_error "Auto-install is only supported on macOS via Homebrew."
        log_error "Please manually install Go and Docker for your OS."
        return 1
    fi

    if ! check_command brew; then
        log_error "Homebrew is not installed. Please install it first: https://brew.sh/"
        return 1
    fi

    if ! check_command go; then
        log_info "Installing Go..."
        brew install go
    fi

    if ! check_command docker; then
        log_info "Installing Docker..."
        brew install --cask docker
    fi
    
    log_success "Installation complete. Please verify with './ail.sh check'."
}

run_checks() {
    log_info "Running pre-flight checks..."
    local errors=0
    
    if check_go; then :; else ((errors++)); fi
    if check_docker; then :; else ((errors++)); fi
    
    if [ $errors -eq 0 ]; then
        log_success "All systems go."
    else
        log_warn "Some checks failed. You may need to run './ail.sh install' or install manually."
        return 1
    fi
}

# --- Local Governance ---

local_start() {
    ensure_state_dir
    
    if [ -f "$PID_FILE" ]; then
        local pid
        pid=$(cat "$PID_FILE")
        if ps -p "$pid" > /dev/null; then
            log_warn "Server running locally with PID $pid."
            return
        else
            log_warn "Removing stale PID file."
            rm "$PID_FILE"
        fi
    fi

    if ! check_go; then
        log_error "Cannot start: Go is missing."
        exit 1
    fi

    log_info "Building binary..."
    if ! go build -o "$BINARY_NAME" ./cmd/server; then
        log_error "Build failed."
        exit 1
    fi
    log_success "Build successful."

    log_info "Starting server..."
    nohup ./$BINARY_NAME > "$LOG_FILE" 2>&1 &
    local new_pid=$!
    echo "$new_pid" > "$PID_FILE"
    
    log_success "Started (PID: $new_pid). Logs at $LOG_FILE"
}

local_stop() {
    if [ ! -f "$PID_FILE" ]; then
        log_warn "No PID file found."
        return
    fi

    local pid
    pid=$(cat "$PID_FILE")
    if ps -p "$pid" > /dev/null; then
        log_info "Stopping server (PID $pid)..."
        kill "$pid"
        rm "$PID_FILE"
        log_success "Stopped."
    else
        log_warn "Process not running. Cleaning PID file."
        rm "$PID_FILE"
    fi
}

local_logs() {
    if [ ! -f "$LOG_FILE" ]; then
        log_error "No log file found at $LOG_FILE"
        exit 1
    fi
    if [ "$1" == "follow" ]; then
        tail -f "$LOG_FILE"
    else
        tail -n 50 "$LOG_FILE"
    fi
}

# --- Docker Governance ---

docker_start() {
    if ! check_docker; then exit 1; fi
    log_info "Starting via Docker Compose..."
    
    # Export current version info for Docker build
    export VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')"
    export COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
    export BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    
    local docker_cmd="docker compose up -d"
    if [ "$1" == "true" ]; then
        docker_cmd="$docker_cmd --build"
    fi
    docker_cmd="$docker_cmd --remove-orphans"

    log_info "Executing: $docker_cmd"
    $docker_cmd
    log_success "Docker containers started."
}

docker_stop() {
    log_info "Stopping Docker containers..."
    docker compose down
    log_success "Docker containers stopped."
}

docker_logs() {
    if [ "$1" == "follow" ]; then
        docker compose logs -f
    else
        docker compose logs --tail=50
    fi
}

# --- Bridge Governance ---

bridge_start() {
    ensure_state_dir
    
    if [ -f "$BRIDGE_PID_FILE" ]; then
        local pid
        pid=$(cat "$BRIDGE_PID_FILE")
        if ps -p "$pid" > /dev/null; then
            log_warn "Bridge agent running with PID $pid."
            return
        else
            log_warn "Removing stale Bridge PID file."
            rm "$BRIDGE_PID_FILE"
        fi
    fi

    if ! check_go; then
        log_error "Cannot start bridge: Go is missing."
        exit 1
    fi

    log_info "Building bridge agent..."
    if ! go build -o "$BRIDGE_BINARY" ./cmd/bridge-agent; then
        log_error "Bridge build failed."
        exit 1
    fi
    log_success "Bridge build successful."

    log_info "Starting bridge agent..."
    nohup ./$BRIDGE_BINARY > "$BRIDGE_LOG_FILE" 2>&1 &
    local new_pid=$!
    echo "$new_pid" > "$BRIDGE_PID_FILE"
    
    log_success "Bridge started (PID: $new_pid). Logs at $BRIDGE_LOG_FILE"
}

bridge_stop() {
    if [ ! -f "$BRIDGE_PID_FILE" ]; then
        log_warn "No Bridge PID file found."
        return
    fi

    local pid
    pid=$(cat "$BRIDGE_PID_FILE")
    if ps -p "$pid" > /dev/null; then
        log_info "Stopping bridge agent (PID $pid)..."
        kill "$pid"
        rm "$BRIDGE_PID_FILE"
        log_success "Bridge stopped."
    else
        log_warn "Bridge process not running. Cleaning PID file."
        rm "$BRIDGE_PID_FILE"
    fi
}

bridge_status() {
     ensure_state_dir
     local pid=""
     if [ -f "$BRIDGE_PID_FILE" ]; then 
        pid=$(cat "$BRIDGE_PID_FILE")
     else
        # Try to find the bridge-agent process if PID file is missing (e.g. running via launchd)
        pid=$(pgrep "bridge-agent" | head -n 1)
     fi

     if [ -n "$pid" ] && ps -p "$pid" > /dev/null; then
        log_success "Bridge: Running (PID $pid)"
     else
        echo "Bridge: Not running."
     fi
}

# --- Router ---

show_help() {
    echo "Usage: ./ail.sh [COMMAND] [OPTIONS]"
    echo ""
    echo "Commands:"
    echo "  start    Build and start the application"
    echo "  stop     Stop the running application"
    echo "  restart  Restart the application"
    echo "  status   Show status of local/docker/bridge instances"
    echo "  bridge   Manage the bridge agent (start|stop|status)"
    echo "  logs     Tail application logs"
    echo "  check    Verify dependencies"
    echo "  install  Install missing dependencies (macOS Only)"
    echo "  help     Show this message"
    echo ""
    echo "Options:"
    echo "  -d, --docker   Target Docker runtime (default: Local)"
    echo "  -b, --build    Rebuild Docker images before starting (only with -d)"
    echo "  -f, --follow   Follow log output (works with start/logs)"
    echo ""
    echo "Examples:"
    echo "  ./ail.sh start               # Start locally in background"
    echo "  ./ail.sh start -f            # Start locally and follow logs"
    echo "  ./ail.sh start -d            # Start in Docker (detached)"
    echo "  ./ail.sh start -d -f         # Start in Docker and follow logs"
    echo "  ./ail.sh logs -f             # Follow logs of running local instance"
    echo "  ./ail.sh check               # Check if Go/Docker are installed"
}

main() {
    local command=""
    local use_docker=false
    local force_build=false
    local follow_logs=false

    # Parse args
    while [[ $# -gt 0 ]]; do
        case $1 in
            start|stop|restart|status|logs|check|install|help|bridge)
                command=$1
                shift
                ;;
            -d|--docker)
                use_docker=true
                shift
                ;;
            -b|--build)
                force_build=true
                shift
                ;;
            -f|--follow)
                follow_logs=true
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                echo "Unknown argument: $1"
                show_help
                exit 1
                ;;
        esac
    done

    if [ -z "$command" ]; then
        show_help
        exit 1
    fi

    case "$command" in
        check)
            run_checks
            ;;
        install)
            install_dependencies
            ;;
        start)
            if $use_docker; then docker_start "$force_build"; else local_start; fi
            if $follow_logs; then
                echo ""
                log_info "Tailing logs... (Press Ctrl+C to detach)"
                if $use_docker; then docker_logs "follow"; else local_logs "follow"; fi
            fi
            ;;
        stop)
            if $use_docker; then docker_stop; else local_stop; fi
            ;;
        restart)
            if $use_docker; then
                docker_stop
                docker_start
            else
                local_stop
                local_start
            fi
            ;;
        logs)
             local mode="tail"
             if $follow_logs; then mode="follow"; fi
             
             if $use_docker; then 
                docker_logs "$mode"
             else 
                local_logs "$mode"
             fi
             ;;
        bridge)
             local sub=$1
             case "$sub" in
                start) bridge_start ;;
                stop) bridge_stop ;;
                status|*) bridge_status ;;
             esac
             ;;
        status)
             ensure_state_dir
             echo "--- Local Status ---"
             if [ -f "$PID_FILE" ]; then 
                pid=$(cat "$PID_FILE")
                if ps -p "$pid" > /dev/null; then
                    log_success "Running (PID $pid)"
                else
                    log_warn "PID file exists but process dead."
                fi
             else
                echo "Not running locally."
             fi
             
             echo ""
             echo "--- Docker Status ---"
             if check_command docker; then
                 docker compose ps
             else
                 echo "Docker not available."
             fi
             
             echo ""
             bridge_status
             ;;
        help)
            show_help
            ;;
    esac
}

main "$@"
