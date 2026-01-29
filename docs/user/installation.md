# Installation Guide

This guide will walk you through setting up `switchAILocal` on your machine.

## Prerequisites

- [Go](https://go.dev/doc/install) (version 1.24 or later recommended)
- Access to terminal/shell

## Build from Source

1. **Clone the repository**:
    ```bash
    git clone https://github.com/traylinx/switchAILocal.git
    cd switchAILocal
    ```

2.  **Build the binary**:
    ```bash
    go build -o switchAILocal ./cmd/server
    ```

3.  **Verify the build**:
    ```bash
    ./switchAILocal --version
    ```

## Building the Management UI (Optional)

If you wish to build the management dashboard from source:

1.  **Run the UI build script**:
    ```bash
    ./ail_ui.sh
    ```
    This script handles the `npm install` and `npm run build` processes and ensures the compiled dashboard is correctly placed in the `static/` directory.

2.  **Verify the artifact**:
    Check that `static/management.html` has been created.

## Running the Server

To start the server with default settings:

```bash
./switchAILocal
```

The server will start on `http://localhost:18080` by default.

### Using the Unified Operations Hub (`ail.sh`)

We provide a robust Hub Script (`ail.sh`) to manage the entire application lifecycle, including dependencies, Docker, and local execution.

**Common Commands:**

- **Check Environment:** Verifies you have Go, Docker, etc. installed.
  ```bash
  ./ail.sh check
  ```

- **Install Dependencies (macOS):** Auto-installs Go/Docker via Homebrew if missing.
  ```bash
  ./ail.sh install
  ```

- **Start (Local):** Builds and runs the app natively in the background.
  ```bash
  ./ail.sh start
  # Or start and follow logs:
  ./ail.sh start -f
  ```

- **Start (Docker):** Builds and runs the app inside Docker.
  ```bash
  ./ail.sh start --docker
  # Or start and follow logs:
  ./ail.sh start --docker -f
  ```

- **Stop:** Stops the running instance (detects mode automatically).
  ```bash
  ./ail.sh stop
  ```

- **Status:** Shows the status of local and Docker instances.
  ```bash
  ./ail.sh status
  ```

- **Logs:** Tails the logs.
  ```bash
  ./ail.sh logs -f
  ```

### Default Configuration
On the first run, the server will look for a `config.yaml` in the current directory. You can use the provided template:

```bash
cp config.example.yaml config.yaml
```

## Docker Installation

### Using docker-build.sh (Recommended)

The easiest way to run with Docker:

```bash
./docker-build.sh
# Choose option 1 for pre-built image
# Choose option 2 to build from source
```

### Manual Docker Commands

Build and run manually:

```bash
docker build -t switchailocal .
docker run -p 18080:18080 -v ./config.yaml:/app/config.yaml switchailocal
```

### Docker Compose

Use the provided `docker-compose.yml`:

```bash
docker compose up -d
```

This mounts:
- `./config.yaml` → `/app/config.yaml`
- `./auths` → `/root/.switchailocal` (auth credentials)
- `./logs` → `/app/logs`
- `./plugins` → `/app/plugins`

