#!/usr/bin/env bash
# =============================================================================
# deploy.sh — run this on the air-gapped host
#
# Prerequisites on the target host:
#   - Docker Engine installed (no internet needed after this)
#   - docker compose plugin (v2)
#
# Steps:
#   1. Copy the entire deployment/ folder to the host
#   2. cp config.example.yaml config.yaml && edit config.yaml
#   3. cp .env.example .env              && edit .env (set SECRET_KEY)
#   4. bash deploy.sh
# =============================================================================

set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'
BOLD='\033[1m'; RESET='\033[0m'

info()    { echo -e "${CYAN}==>${RESET} ${BOLD}$*${RESET}"; }
success() { echo -e "${GREEN}✔${RESET}  $*"; }
die()     { echo -e "${RED}✘  $*${RESET}" >&2; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IMAGES_DIR="${SCRIPT_DIR}/images"

# ── Pre-flight ────────────────────────────────────────────────────────────────
command -v docker &>/dev/null || die "docker not found"
docker info &>/dev/null       || die "Docker daemon not running"

[[ -f "${SCRIPT_DIR}/config.yaml" ]]  || die "config.yaml not found — copy config.example.yaml and edit it"
[[ -f "${SCRIPT_DIR}/.env" ]]         || die ".env not found — copy .env.example and set SECRET_KEY"
[[ -f "${IMAGES_DIR}/flowgate.tar.gz" ]] || die "images/flowgate.tar.gz not found"
[[ -f "${IMAGES_DIR}/minio.tar.gz" ]]        || die "images/minio.tar.gz not found"

# ── Load images ───────────────────────────────────────────────────────────────
info "Loading Docker images (this may take a minute)"

info "  Loading flowgate"
docker load < "${IMAGES_DIR}/flowgate.tar.gz"
success "  flowgate loaded"

info "  Loading minio"
docker load < "${IMAGES_DIR}/minio.tar.gz"
success "  minio loaded"

# ── Start services ────────────────────────────────────────────────────────────
info "Starting services with docker compose"
cd "${SCRIPT_DIR}"
docker compose --env-file .env up -d --remove-orphans

# ── Wait for health ───────────────────────────────────────────────────────────
info "Waiting for flowgate to become healthy"
ATTEMPTS=0
MAX=24
until docker compose ps flowgate | grep -q "healthy" || [[ ${ATTEMPTS} -ge ${MAX} ]]; do
    sleep 5
    ATTEMPTS=$((ATTEMPTS + 1))
    echo "  ... waiting (${ATTEMPTS}/${MAX})"
done

if [[ ${ATTEMPTS} -ge ${MAX} ]]; then
    echo ""
    die "flowgate did not become healthy in time — check: docker compose logs flowgate"
fi

success "flowgate is healthy"

# ── Done ──────────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}${BOLD}Deployment complete.${RESET}"
echo ""
PORT="$(grep FLOWGATE_PORT .env 2>/dev/null | cut -d= -f2 || echo 8080)"
echo "  Dashboard : http://$(hostname -I | awk '{print $1}'):${PORT:-8080}"
echo "  Health    : http://$(hostname -I | awk '{print $1}'):${PORT:-8080}/health"
echo "  Logs      : docker compose logs -f flowgate"
echo ""
