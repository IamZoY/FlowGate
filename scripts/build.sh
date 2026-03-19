#!/usr/bin/env bash
set -euo pipefail

APP="flowgate"
VERSION="${VERSION:-$(cat VERSION 2>/dev/null || echo "0.0.0")}"
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_DIRTY=$(git diff --quiet 2>/dev/null && echo "" || echo "-dirty")
BUILD_TIME=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=${GIT_COMMIT}${GIT_DIRTY} -X main.buildTime=${BUILD_TIME}"

DIST="dist"
rm -rf "${DIST}"
mkdir -p "${DIST}"

# Target platforms: OS/ARCH
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
)

echo "=== Building ${APP} v${VERSION} (${GIT_COMMIT}${GIT_DIRTY}) ==="
echo ""

for PLATFORM in "${PLATFORMS[@]}"; do
    GOOS="${PLATFORM%/*}"
    GOARCH="${PLATFORM#*/}"
    OUTPUT="${APP}-${GOOS}-${GOARCH}"

    if [ "${GOOS}" = "windows" ]; then
        OUTPUT="${OUTPUT}.exe"
    fi

    echo "  Building ${OUTPUT}..."
    CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" \
        go build -ldflags="${LDFLAGS}" -trimpath -o "${DIST}/${OUTPUT}" ./cmd/flowgate

    echo "    ✓ $(du -h "${DIST}/${OUTPUT}" | cut -f1 | xargs) ${OUTPUT}"
done

echo ""
echo "=== Generating checksums ==="

cd "${DIST}"
if command -v sha256sum &>/dev/null; then
    sha256sum * > checksums.txt
elif command -v shasum &>/dev/null; then
    shasum -a 256 * > checksums.txt
else
    echo "WARNING: No sha256sum or shasum found, skipping checksums"
fi
cd ..

echo ""
echo "=== Build complete ==="
echo ""
echo "Artifacts in ${DIST}/:"
ls -lh "${DIST}/"
