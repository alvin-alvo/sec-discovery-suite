#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-v1.0.0}"
ARCH="linux-amd64"
DIST_DIR="dist"
RELEASE_NAME="sds-${VERSION}-${ARCH}"
STAGE_DIR="${DIST_DIR}/${RELEASE_NAME}"

echo "=== Packaging sec-discovery-suite Release: ${VERSION} (${ARCH}) ==="

mkdir -p "${DIST_DIR}"
rm -rf "${STAGE_DIR}"
mkdir -p "${STAGE_DIR}/bin" "${STAGE_DIR}/scripts" "${STAGE_DIR}/docs/images"

# 1. Ensure binaries are built
if [ ! -f "bin/sds-agent" ] || [ ! -f "bin/sds-server" ]; then
    echo "Binaries missing. Compiling with 'make all'..."
    make all
fi

# 2. Check agent size budget
./scripts/check-size.sh bin/sds-agent 4500000

# 3. Copy binaries
cp -p bin/sds-agent "${STAGE_DIR}/bin/sds-agent"
cp -p bin/sds-server "${STAGE_DIR}/bin/sds-server"
chmod +x "${STAGE_DIR}/bin/"*

# 4. Copy scripts and docs
cp -p scripts/gen-traffic.sh "${STAGE_DIR}/scripts/" 2>/dev/null || true
cp -p scripts/caps.sh "${STAGE_DIR}/scripts/" 2>/dev/null || true
cp -p scripts/check-size.sh "${STAGE_DIR}/scripts/" 2>/dev/null || true
chmod +x "${STAGE_DIR}/scripts/"*.sh 2>/dev/null || true

cp -p docs/images/*.png "${STAGE_DIR}/docs/images/" 2>/dev/null || true
cp -p LICENSE "${STAGE_DIR}/"
cp -p README.md "${STAGE_DIR}/"
cp -p RELEASE.md "${STAGE_DIR}/"

# 5. Create one-click standalone launcher in root of archive
cat << 'EOF' > "${STAGE_DIR}/start.sh"
#!/usr/bin/env bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR"

echo "=========================================================="
echo " Starting Cryptographic Discovery Suite (v1.0.0)"
echo "=========================================================="

mkdir -p data

# Check if agent requires sudo or has capabilities
if [ "$(id -u)" -ne 0 ]; then
    echo "Note: The agent requires raw socket and process inspection permissions."
    echo "Launching agent with sudo..."
    SUDO_CMD="sudo"
else
    SUDO_CMD=""
fi

# Start Agent
$SUDO_CMD ./bin/sds-agent serve --bind 127.0.0.1:7471 &
AGENT_PID=$!

sleep 1

# Start Server
./bin/sds-server --bind 127.0.0.1:8080 --agent http://127.0.0.1:7471 &
SERVER_PID=$!

trap "kill $AGENT_PID $SERVER_PID 2>/dev/null || true" EXIT INT TERM

sleep 2

echo ""
echo "=========================================================="
echo " Cryptographic Discovery Suite is RUNNING!"
echo " Web UI Dashboard: http://127.0.0.1:8080"
echo " Agent Telemetry:  http://127.0.0.1:7471"
echo " (Press Ctrl+C to terminate both services)"
echo "=========================================================="
echo ""

# Try opening default browser
if which xdg-open >/dev/null 2>&1; then
    xdg-open http://127.0.0.1:8080 >/dev/null 2>&1 || true
fi

wait $SERVER_PID
EOF
chmod +x "${STAGE_DIR}/start.sh"

# 6. Copy standalone binaries directly to dist/ for individual download
cp -p bin/sds-agent "${DIST_DIR}/sds-agent"
cp -p bin/sds-server "${DIST_DIR}/sds-server"
chmod +x "${DIST_DIR}/sds-agent" "${DIST_DIR}/sds-server"

# 7. Create tar.gz archive
TARBALL="${DIST_DIR}/${RELEASE_NAME}.tar.gz"
echo "Creating archive: ${TARBALL}..."
tar -czf "${TARBALL}" -C "${DIST_DIR}" "${RELEASE_NAME}"

# 8. Generate GitHub Release Notes
cat << 'EOF' > "${DIST_DIR}/RELEASE_NOTES.md"
# Cryptographic Discovery Suite v1.0.0 (Linux x86_64)

A high-performance cryptographic discovery and network telemetry platform engineered for Linux systems.

## Key Capabilities
- **Passive TLS Network Sniffer:** Linux kernel `AF_PACKET` raw socket capture decoding TLS 1.2 and TLS 1.3 handshakes (ClientHello/ServerHello) in real time without terminating connections or decrypting payload data.
- **Filesystem Certificate Auditing:** Multi-threaded discovery across system trust stores (`/etc/ssl`, `/usr/share/ca-certificates`, etc.) and custom paths, decoding X.509 certificates and deduplicating by SHA-256 fingerprint.
- **Cryptographic Bill of Materials (CBOM) Graph:** Interactive DAG mapping `Host` -> `Process` -> `Endpoint` -> `Certificate` with high-contrast zoom controls and risk indicators.
- **Zero-Decryption Guarantee:** Strictly metadata-only; never stores, logs, or transmits private keys or plaintext application payload bytes.
- **Minimal Agent Footprint:** Statically linked Go binary compressed with UPX (<= 2.44 MB).
- **Embedded Web UI & In-App Documentation:** Full operational dashboard with embedded DuckDB analytical database and comprehensive in-app architecture/legal guide ("i" button).

## Quickstart (Run in 60 seconds)
```bash
# 1. Download and extract
curl -sLO https://github.com/alvin-alvo/sec-discovery-suite/releases/download/v1.0.0/sds-v1.0.0-linux-amd64.tar.gz
tar -xzf sds-v1.0.0-linux-amd64.tar.gz
cd sds-v1.0.0-linux-amd64

# 2. Launch
./start.sh
```

## Release Assets
- `sds-v1.0.0-linux-amd64.tar.gz`: Complete standalone package with `./start.sh` launcher.
- `sds-agent`: Lightweight standalone agent binary.
- `sds-server`: Standalone server binary with embedded DuckDB and Web UI.
- `SHA256SUMS`: Verification checksums.

## Legal & Copyright
Copyright (C) 2026 Alvin Alvo (https://github.com/alvin-alvo/sec-discovery-suite). All rights reserved.
Distributed under the GNU General Public License v3.0 (GPLv3).
EOF

# 9. Generate checksums for all release assets
cd "${DIST_DIR}"
sha256sum "${RELEASE_NAME}.tar.gz" sds-agent sds-server > SHA256SUMS
cd ..

echo "=== Release Packaging Complete ==="
ls -lh "${DIST_DIR}/${RELEASE_NAME}.tar.gz" "${DIST_DIR}/sds-agent" "${DIST_DIR}/sds-server"
echo ""
echo "=== SHA-256 Checksums ==="
cat "${DIST_DIR}/SHA256SUMS"

