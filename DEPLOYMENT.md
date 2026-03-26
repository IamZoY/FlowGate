# FlowGate Deployment Guide

## Overview

FlowGate is a lightweight S3 object transfer gateway. It receives webhook events from MinIO, streams objects from source to destination buckets, and provides a real-time dashboard for monitoring all transfers. The deployment model supports fully air-gapped environments.

**Architecture:**
```
MinIO Source  ──webhook──▶  FlowGate  ──S3 PutObject──▶  MinIO Destination
                              │
                         SQLite DB
                         Web Dashboard (HTTPS :443)
```

---

## 1. Prerequisites

### Build Machine (internet-connected)

- Docker Engine 20+
- Git

### Target Host (can be air-gapped)

- Docker Engine 20+
- Docker Compose v2 plugin (`docker compose`, not `docker-compose`)
- `openssl` (auto-generates TLS certificates and SECRET_KEY)
- Disk space: ~500MB for images + data volumes
- No internet required after deployment

### Post-Deployment Configuration

- `mc` (MinIO Client) — needed to configure webhooks on MinIO instances
- Install: `brew install minio/stable/mc` (macOS) or download from https://dl.min.io/client/mc/release/

---

## 2. Building the Deployment Package

On an internet-connected machine:

```bash
git clone <repo-url>
cd flowgate

# Build with auto-version from git SHA
bash build.sh

# Or specify a version
VERSION=1.0.0 bash build.sh
```

`build.sh` produces:

```
deployment/
├── images/
│   ├── flowgate.tar.gz      # FlowGate Docker image (~5MB)
│   └── minio.tar.gz         # MinIO Docker image (~47MB)
├── docker-compose.yml       # Production compose (HTTPS, pre-loaded images)
├── config.yaml              # Production config (TLS enabled, ready to use)
├── .env                     # Environment variables (SECRET_KEY auto-generated)
├── deploy.sh                # Fully automatic deployment script
└── manifest.txt             # Build metadata + SHA256 checksums
```

Package for transfer:

```bash
tar -czf flowgate-deployment.tar.gz deployment/
```

Transfer the tarball to the air-gapped host via USB, SCP, or any file transfer method.

---

## 3. Deploying on the Target Host

```bash
tar -xzf flowgate-deployment.tar.gz
cd deployment
bash deploy.sh
```

`deploy.sh` automatically:
1. Generates `SECRET_KEY` in `.env` (if still a placeholder)
2. Generates self-signed TLS certificate + key in `certs/` (valid for 10 years)
3. Loads Docker images from gzipped tarballs
4. Starts services with `docker compose up -d`
5. Waits for the health check to pass (up to 2 minutes)
6. Prints the HTTPS dashboard URL

**IMPORTANT:** After first run, back up your `.env` file — the `SECRET_KEY` is required to decrypt MinIO credentials stored in SQLite. If you lose it, all stored credentials become unrecoverable.

### Optional Customization

You can edit these files **before** running `deploy.sh`:

| File | What to Change |
|------|---------------|
| `config.yaml` | Worker pool size, queue capacity, retry settings, logging |
| `.env` | `FLOWGATE_PORT` (default: 443), `LOG_LEVEL` (default: info) |

To run without TLS, remove or comment out `tls_cert` and `tls_key` in `config.yaml` and change the port to `8080`.

---

## 4. Post-Deployment Setup

### 4a. Create a Group

Groups are logical groupings for apps.

```bash
PROXY=https://<host>

curl -s -X POST ${PROXY}/api/groups \
  -H 'Content-Type: application/json' \
  -d '{"name": "site-a", "description": "Site A data feeds"}'
```

Save the `id` from the response.

### 4b. Create an App

Each app defines one source MinIO → one destination MinIO transfer pipeline.

```bash
GROUP_ID="<group-id-from-above>"

curl -s -X POST ${PROXY}/api/groups/${GROUP_ID}/apps \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "sensor-data",
    "description": "Raw sensor CSV files",
    "src": {
      "endpoint": "<source-minio-host>:9000",
      "access_key": "<src-access-key>",
      "secret_key": "<src-secret-key>",
      "bucket": "<source-bucket>",
      "region": "us-east-1",
      "use_ssl": false
    },
    "dst": {
      "endpoint": "<dest-minio-host>:9000",
      "access_key": "<dst-access-key>",
      "secret_key": "<dst-secret-key>",
      "bucket": "<dest-bucket>",
      "region": "us-east-1",
      "use_ssl": false
    }
  }'
```

Save the `id` from the response.

**Note:** MinIO endpoint hostnames must be reachable from the flowgate container. If MinIO runs on the same Docker network, use the Docker service name. If external, use the IP or hostname.

### 4c. Get the Webhook Secret

```bash
APP_ID="<app-id-from-above>"
curl -s ${PROXY}/api/apps/${APP_ID}/webhook-url
```

Response:
```json
{
  "app_id": "...",
  "webhook_secret": "0c28362f147595451bb4653ec3707cfd..."
}
```

Save the `webhook_secret` — you'll need it for MinIO configuration.

### 4d. Configure MinIO Webhooks

On a machine with `mc` installed and network access to the source MinIO:

**Step 1 — Set alias:**
```bash
mc alias set mysrc http://<source-minio>:9000 <access-key> <secret-key>
```

**Step 2 — Register webhook target:**
```bash
mc admin config set mysrc notify_webhook:<identifier> \
  endpoint="https://<flowgate-host>/webhook/<group-name>/<app-name>" \
  auth_token="<webhook_secret>"
```

- `<identifier>` is an arbitrary name (e.g., `site-a-sensors`)
- `<group-name>` and `<app-name>` are the **name slugs**, not UUIDs
- `auth_token` is the `webhook_secret` from step 4c

**Step 3 — Restart MinIO:**
```bash
mc admin service restart mysrc
# Or: docker restart <minio-container>
```

**Step 4 — Bind event to bucket:**
```bash
mc event add mysrc/<source-bucket> \
  arn:minio:sqs::<identifier>:webhook \
  --event put
```

**Step 5 — Verify:**
```bash
mc event list mysrc/<source-bucket>
```

Expected output:
```
arn:minio:sqs::<identifier>:webhook   s3:ObjectCreated:*   Filter:
```

### 4e. Test the Flow

```bash
# Upload a test file to the source bucket
echo "test" > /tmp/test.txt
mc cp /tmp/test.txt mysrc/<source-bucket>/test.txt

# Check the transfer status
curl -s ${PROXY}/api/transfers | python3 -m json.tool

# Verify file in destination
mc ls mydst/<dest-bucket>/
```

### 4f. Multiple Apps

Repeat steps 4a–4d for each source→destination pipeline. Each app gets its own webhook URL and secret:

```
/webhook/site-a/sensor-data     →  App 1
/webhook/site-a/video-feeds     →  App 2
/webhook/site-b/audit-logs      →  App 3
```

---

## 5. Monitoring

### Dashboard

Open `https://<host>` in a browser (accept the self-signed certificate warning). The dashboard shows:

- **Stats bar:** Total transfers, success/failed counts, queued jobs, total bytes
- **Live feed:** Real-time transfer events via WebSocket
- **Group/App management:** Create, view, and delete pipelines

### Health Check

```bash
curl -k https://<host>/health
# {"status":"ok","queue_depth":0}
```

A non-zero `queue_depth` means transfers are queued or in-progress.

### Stats API

```bash
curl -k https://<host>/api/stats
```

Returns aggregate counts: `total_transfers`, `success_count`, `failed_count`, `in_progress_count`, `pending_count`, `total_bytes`, `avg_duration_ms`.

### Logs

```bash
docker compose logs -f flowgate
```

Logs are JSON-formatted by default with structured fields: `transfer_id`, `app_id`, `object_key`, `duration_ms`, `bytes`.

The production compose configures log rotation: max 50MB per file, 5 files.

---

## 6. Backup & Restore

### What to Back Up

| Item | Location | Purpose |
|------|----------|---------|
| SQLite database | Docker volume `flowgate-data` at `/data/flowgate.db` | All groups, apps, encrypted credentials, transfer history |
| `.env` file | `deployment/.env` | SECRET_KEY (required to decrypt credentials) |
| `config.yaml` | `deployment/config.yaml` | Service configuration |
| TLS certificates | `deployment/certs/` | Self-signed cert + private key |

### Backup Procedure

```bash
# Stop the proxy for a consistent backup
docker compose stop flowgate

# Copy DB from the volume
docker run --rm \
  -v deployment_flowgate-data:/data \
  -v $(pwd):/backup \
  alpine cp /data/flowgate.db /backup/flowgate-$(date +%Y%m%d).db

# Start again
docker compose start flowgate
```

### Restore Procedure

```bash
docker compose stop flowgate

docker run --rm \
  -v deployment_flowgate-data:/data \
  -v $(pwd):/backup \
  alpine cp /backup/flowgate-YYYYMMDD.db /data/flowgate.db

docker compose start flowgate
```

**IMPORTANT:** The `SECRET_KEY` in `.env` must be the same key that was used when the database was created. If you change the key, all encrypted MinIO credentials in the database become unreadable.

---

## 7. Upgrading

### On the Build Machine

```bash
git pull
VERSION=1.1.0 bash build.sh
tar -czf flowgate-deployment-1.1.0.tar.gz deployment/
```

### On the Target Host

```bash
# Back up your config and secrets first
cp deployment/.env deployment/.env.bak
cp deployment/config.yaml deployment/config.yaml.bak

# Extract new package (overwrites images and scripts)
tar -xzf flowgate-deployment-1.1.0.tar.gz

cd deployment

# Restore your config and secrets
cp .env.bak .env
cp config.yaml.bak config.yaml

# deploy.sh loads new images and recreates containers
bash deploy.sh
```

The SQLite database is in a Docker volume and survives container recreation. Schema migrations run automatically on startup.

---

## 8. Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Transfers failing with credential errors | MinIO credentials encrypted in DB, decryption failing | Verify `SECRET_KEY` in `.env` matches the original key |
| Webhooks not arriving (no transfers queued) | MinIO can't reach flowgate endpoint | Check network connectivity, verify `mc event list`, check MinIO logs |
| 401 Unauthorized on webhook | `auth_token` doesn't match `webhook_secret` | Re-get the secret via API, update MinIO config, restart MinIO |
| 503 Service Unavailable | Transfer queue full | Increase `transfer.queue_capacity` or `transfer.worker_pool_size` in config.yaml |
| Slow transfers | Worker pool too small | Increase `transfer.worker_pool_size` (default: 10) |
| Browser certificate warning | Self-signed TLS certificate | Expected — add exception in browser or import `certs/flowgate.crt` |
| Connection refused on port 443 | Port conflict or firewall | Check `FLOWGATE_PORT` in `.env`, verify no other service on 443 |
| Dashboard not loading | Container not running or port blocked | `docker compose ps`, check firewall rules |
| WebSocket disconnecting | Proxy restarting or network issue | Check proxy logs, verify health endpoint |

### Useful Commands

```bash
# Service status
docker compose ps

# Live logs
docker compose logs -f flowgate

# Recent transfers
curl -sk https://<host>/api/transfers?limit=10 | python3 -m json.tool

# Failed transfers only
curl -sk "https://<host>/api/transfers?status=failed" | python3 -m json.tool

# Stats
curl -sk https://<host>/api/stats | python3 -m json.tool

# List all groups
curl -sk https://<host>/api/groups | python3 -m json.tool

# List apps in a group
curl -sk https://<host>/api/groups/<id>/apps | python3 -m json.tool
```

---

## API Quick Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/api/groups` | List all groups |
| `POST` | `/api/groups` | Create group |
| `GET` | `/api/groups/{id}` | Get group |
| `PUT` | `/api/groups/{id}` | Update group |
| `DELETE` | `/api/groups/{id}` | Delete group |
| `GET` | `/api/groups/{id}/apps` | List apps in group |
| `POST` | `/api/groups/{id}/apps` | Create app |
| `GET` | `/api/apps/{id}` | Get app |
| `PUT` | `/api/apps/{id}` | Update app |
| `DELETE` | `/api/apps/{id}` | Delete app |
| `GET` | `/api/apps/{id}/webhook-url` | Get webhook URL and secret |
| `GET` | `/api/apps/{id}/transfers` | List transfers for app |
| `GET` | `/api/transfers` | List all transfers (filterable) |
| `GET` | `/api/transfers/{id}` | Get transfer details |
| `GET` | `/api/stats` | Aggregate transfer statistics |
| `GET` | `/ws` | WebSocket live feed |
| `POST` | `/webhook/{group}/{app}` | Webhook endpoint (MinIO → proxy) |
