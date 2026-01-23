# docker-build.ps1 - Windows PowerShell Build Script for switchAILocal
#
# This script automates the process of building and running the Docker container
# with version information dynamically injected at build time.

# Stop script execution on any error
$ErrorActionPreference = "Stop"

# --- Step 1: Choose Environment ---
Write-Host "switchAILocal Docker Builder"
Write-Host "============================"
Write-Host ""
Write-Host "Please select an option:"
Write-Host "1) Run using Pre-built Image (Recommended)"
Write-Host "2) Build from Source and Run (For Developers)"
$choice = Read-Host -Prompt "Enter choice [1-2]"

# --- Step 2: Execute based on choice ---
switch ($choice) {
    "1" {
        Write-Host ""
        Write-Host "--- Running with Pre-built Image ---"
        docker compose up -d --remove-orphans --no-build
        Write-Host ""
        Write-Host "switchAILocal is starting from remote image."
        Write-Host "Run 'docker compose logs -f' to see the logs."
        Write-Host "Access the API at http://localhost:18080"
    }
    "2" {
        Write-Host ""
        Write-Host "--- Building from Source and Running ---"

        # Get Version Information
        try {
            $VERSION = (git describe --tags --always --dirty)
        } catch {
            $VERSION = "dev"
        }
        try {
            $COMMIT = (git rev-parse --short HEAD)
        } catch {
            $COMMIT = "unknown"
        }
        $BUILD_DATE = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

        Write-Host "Building with the following info:"
        Write-Host "  Version: $VERSION"
        Write-Host "  Commit: $COMMIT"
        Write-Host "  Build Date: $BUILD_DATE"
        Write-Host "----------------------------------------"

        # Build and start the services with a local-only image tag
        $env:SWITCH_AI_IMAGE = "switchailocal:local"
        
        Write-Host "Building the Docker image..."
        docker compose build --build-arg VERSION=$VERSION --build-arg COMMIT=$COMMIT --build-arg BUILD_DATE=$BUILD_DATE

        Write-Host "Starting the services..."
        docker compose up -d --remove-orphans --pull never

        Write-Host ""
        Write-Host "Build complete. switchAILocal is starting."
        Write-Host "Run 'docker compose logs -f' to see the logs."
        Write-Host "Access the API at http://localhost:18080"
    }
    default {
        Write-Host "Invalid choice. Please enter 1 or 2."
        exit 1
    }
}