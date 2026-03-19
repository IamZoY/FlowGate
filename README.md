# FlowGate

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/IamZoY/FlowGate?color=green)](https://github.com/IamZoY/FlowGate/releases)
[![Downloads](https://img.shields.io/github/downloads/IamZoY/FlowGate/total?color=purple)](https://github.com/IamZoY/FlowGate/releases)
[![Build](https://github.com/IamZoY/FlowGate/actions/workflows/ci.yml/badge.svg)](https://github.com/IamZoY/FlowGate/actions/workflows/ci.yml)
[![Release](https://github.com/IamZoY/FlowGate/actions/workflows/release.yml/badge.svg)](https://github.com/IamZoY/FlowGate/actions/workflows/release.yml)

**Lightweight S3 object transfer gateway for MinIO.** FlowGate receives webhook events when objects are created, streams them from source to destination buckets in real-time, and provides a live web dashboard for monitoring. Purpose-built for air-gapped deployments.

---

## Features

| Category | Feature |
|---|---|
| **Transfer** | Real-time object streaming between MinIO instances |
| **Transfer** | Configurable worker pool with backpressure (queue depth) |
| **Transfer** | Automatic retries with exponential backoff |
| **Security** | AES-256-GCM encryption for stored credentials |
| **Security** | HMAC webhook validation (timing-safe) |
| **Dashboard** | Real-time WebSocket feed (transfers, stats, events) |
| **Dashboard** | Group and app management UI |
| **Operations** | Structured JSON/text logging via `slog` |
| **Operations** | Health check endpoint with queue depth |
| **Operations** | Graceful shutdown (drain HTTP, finish in-flight jobs) |
| **Storage** | SQLite with WAL mode — zero external dependencies |
| **Deployment** | Single static binary (~5 MB Docker image) |
| **Deployment** | Air-gapped packaging with `build.sh` |

## Architecture

```
┌──────────────┐    webhook     ┌───────────┐    stream     ┌──────────────┐
│  Source MinIO │ ────────────► │  FlowGate │ ────────────► │   Dest MinIO │
└──────────────┘               └─────┬─────┘               └──────────────┘
                                     │
                               ┌─────┴─────┐
                               │ Dashboard  │  ◄── WebSocket
                               │  (SPA)     │
                               └───────────┘
```

## Quick Start

### Prerequisites

- Go 1.24+ (build from source) **or** Docker
- One or more MinIO instances

### Option 1: Docker Compose (recommended)

```bash
git clone https://github.com/IamZoY/FlowGate.git
cd flowgate
cp config.example.yaml config.yaml

# Set a strong secret key
export SECRET_KEY=$(openssl rand -hex 32)

docker compose up -d
```

The dashboard is available at `http://localhost:8080`.

### Option 2: Binary

Download a prebuilt binary from the [Releases](https://github.com/IamZoY/FlowGate/releases) page, verify the checksum, and run:

```bash
# Verify checksum (Linux/macOS)
sha256sum -c checksums.txt

# Run
chmod +x flowgate-*
./flowgate-linux-amd64 --config config.yaml
```

### Option 3: Build from Source

```bash
git clone https://github.com/IamZoY/FlowGate.git
cd flowgate
make build
./flowgate --config config.example.yaml
```

## Installation

See the full [Installation Guide](#installation-guide) below.

## Configuration

FlowGate uses a YAML config file with environment variable interpolation (`${VAR}`):

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "120s"

database:
  path: "./flowgate.db"
  max_open_connections: 5
  max_idle_connections: 2

transfer:
  worker_pool_size: 10
  queue_capacity: 1000
  retry_attempts: 3
  retry_backoff: "5s"

logging:
  level: "info"        # debug, info, warn, error
  format: "json"       # json, text

security:
  secret_key: "${SECRET_KEY}"

dashboard:
  enabled: true
  auth_enabled: false
  username: "admin"
  password: "${DASH_PASSWORD}"
```

## Usage

### 1. Create a Group

Groups are logical namespaces for organizing transfer pipelines.

```bash
curl -X POST http://localhost:8080/api/groups \
  -H "Content-Type: application/json" \
  -d '{"name": "production", "description": "Production transfers"}'
```

### 2. Create an App

Apps define a source-to-destination transfer pipeline within a group.

```bash
curl -X POST http://localhost:8080/api/groups/{group_id}/apps \
  -H "Content-Type: application/json" \
  -d '{
    "name": "backup",
    "source": {
      "endpoint": "minio-src:9000",
      "access_key": "minioadmin",
      "secret_key": "minioadmin",
      "bucket": "source-bucket",
      "use_ssl": false
    },
    "destination": {
      "endpoint": "minio-dst:9000",
      "access_key": "minioadmin",
      "secret_key": "minioadmin",
      "bucket": "dest-bucket",
      "use_ssl": false
    }
  }'
```

### 3. Configure MinIO Webhook

Point your source MinIO bucket notification to FlowGate:

```bash
mc event add myminio/source-bucket arn:minio:sqs::1:webhook \
  --event put \
  --suffix "" \
  --prefix ""
```

Set the webhook endpoint in MinIO to: `http://flowgate:8080/webhook/{group}/{app}`

### 4. Monitor

Open the dashboard at `http://localhost:8080` to see transfers in real-time.

## API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check with queue depth |
| `GET` | `/ws` | WebSocket live feed |
| `POST` | `/webhook/{group}/{app}` | MinIO webhook ingest |
| `GET` | `/api/groups` | List groups |
| `POST` | `/api/groups` | Create group |
| `GET` | `/api/groups/{id}` | Get group |
| `PUT` | `/api/groups/{id}` | Update group |
| `DELETE` | `/api/groups/{id}` | Delete group |
| `GET` | `/api/groups/{id}/apps` | List apps in group |
| `POST` | `/api/groups/{id}/apps` | Create app |
| `GET` | `/api/apps/{id}` | Get app |
| `PUT` | `/api/apps/{id}` | Update app |
| `DELETE` | `/api/apps/{id}` | Delete app |
| `GET` | `/api/apps/{id}/webhook-url` | Get webhook URL & secret |
| `GET` | `/api/transfers` | List transfers (filterable) |
| `GET` | `/api/transfers/{id}` | Get transfer |
| `GET` | `/api/stats` | Aggregate statistics |

## Installation Guide

### System Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 1 core | 2+ cores |
| RAM | 128 MB | 512 MB |
| Disk | 50 MB | Depends on DB size |
| OS | Linux, macOS, Windows | Linux (amd64/arm64) |

### Download

Go to the [Releases](https://github.com/IamZoY/FlowGate/releases) page and download the binary for your platform:

| Platform | Binary |
|----------|--------|
| Linux (x86_64) | `flowgate-linux-amd64` |
| Linux (ARM64) | `flowgate-linux-arm64` |
| macOS (Intel) | `flowgate-darwin-amd64` |
| macOS (Apple Silicon) | `flowgate-darwin-arm64` |
| Windows (x86_64) | `flowgate-windows-amd64.exe` |

### Verify Checksum

Each release includes a `checksums.txt` file with SHA-256 hashes:

```bash
# Linux
sha256sum -c checksums.txt

# macOS
shasum -a 256 -c checksums.txt
```

### Run as a Systemd Service (Linux)

```ini
# /etc/systemd/system/flowgate.service
[Unit]
Description=FlowGate S3 Transfer Gateway
After=network.target

[Service]
Type=simple
User=flowgate
Group=flowgate
ExecStart=/usr/local/bin/flowgate --config /etc/flowgate/config.yaml
Restart=on-failure
RestartSec=5
Environment=SECRET_KEY=your-secret-key-here

[Install]
WantedBy=multi-user.target
```

```bash
sudo useradd -r -s /sbin/nologin flowgate
sudo mkdir -p /etc/flowgate
sudo cp config.example.yaml /etc/flowgate/config.yaml
sudo cp flowgate-linux-amd64 /usr/local/bin/flowgate
sudo chmod +x /usr/local/bin/flowgate
sudo systemctl daemon-reload
sudo systemctl enable --now flowgate
```

### Docker

```bash
docker run -d \
  --name flowgate \
  -p 8080:8080 \
  -e SECRET_KEY=$(openssl rand -hex 32) \
  -v $(pwd)/config.yaml:/config.yaml:ro \
  -v flowgate-data:/data \
  ghcr.io/iamzoy/flowgate:latest \
  --config /config.yaml
```

### Air-Gapped Deployment

FlowGate includes tooling for fully offline deployments:

```bash
# On a machine with internet access:
./build.sh

# Transfer the deployment/ directory to the air-gapped environment
# On the target machine:
cd deployment
./deploy.sh
```

## Building from Source

```bash
# Clone
git clone https://github.com/IamZoY/FlowGate.git
cd flowgate

# Build for current platform
make build

# Build for all platforms
make release

# Run tests
make test

# See all targets
make help
```

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

This project is licensed under the MIT License — see [LICENSE](LICENSE) for details.
