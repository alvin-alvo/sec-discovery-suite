# Release & Reviewer Testing Guide

This guide is designed for reviewers and evaluators to test **Cryptographic Discovery Suite (`sec-discovery-suite`)** on an Ubuntu machine in under **3 minutes** without installing Go, Node.js, or compiler toolchains.

---

## 1. GitHub Release Structure

All precompiled assets and official distribution tarballs are available on the [GitHub Latest Release Page](https://github.com/alvin-alvo/sec-discovery-suite/releases/latest).

When downloading or packaging a release, the following artifacts are provided:

| Asset Name | Description | Size |
|---|---|---|
| `sds-v1.0.0-linux-amd64.tar.gz` | Complete standalone release package with `start.sh`, binaries, scripts, and documentation | ~28 MB |
| `sds-agent` | Statically linked Linux x86_64 agent binary (UPX-compressed) | ~2.4 MB |
| `sds-server` | Linux x86_64 server binary with embedded DuckDB & Web UI | ~63 MB |
| `SHA256SUMS` | SHA-256 checksum file for all release artifacts | <1 KB |

---

## 2. Reviewer Plug-and-Play Quickstart (60 Seconds)

Reviewers can test the suite directly on Ubuntu (24.04 or 22.04 LTS) by downloading the latest bundle:

```bash
# 1. Download release bundle from GitHub latest release
curl -sLO https://github.com/alvin-alvo/sec-discovery-suite/releases/latest/download/sds-v1.0.0-linux-amd64.tar.gz

# 2. Verify SHA-256 checksum
echo "d23f85dc95f2108449e99ab63d5d085a9b7924b78b718c2ccd83502673242044  sds-v1.0.0-linux-amd64.tar.gz" | sha256sum --check


# 3. Extract and launch
tar -xzf sds-v1.0.0-linux-amd64.tar.gz
cd sds-v1.0.0-linux-amd64
./start.sh
```

---

## 3. Automated Reviewer Setup Script (Alternative)

Reviewers can run the following bash script on Ubuntu (24.04 or 22.04 LTS) to download, configure, and launch the platform:

```bash
#!/usr/bin/env bash
set -e

# Create a clean test workspace
mkdir -p ~/sds-eval && cd ~/sds-eval

echo "=== 1. Downloading sec-discovery-suite Release Assets ==="
RELEASE_TAG="v1.0.0"
REPO="alvin-alvo/sec-discovery-suite"
BASE_URL="https://github.com/${REPO}/releases/download/${RELEASE_TAG}"

# (If running directly from repository clone, you can use ./bin and ./scripts directly)
if [ ! -f "sds-agent" ] && [ -f "../../bin/sds-agent" ]; then
    cp ../../bin/sds-agent .
    cp ../../bin/sds-server .
    tar -czf demo-suite.tar.gz -C ../.. scripts .test-certs
fi

# Download binaries if not present
if [ ! -f "sds-agent" ]; then
    curl -sLO "${BASE_URL}/sds-agent"
    curl -sLO "${BASE_URL}/sds-server"
    curl -sLO "${BASE_URL}/demo-suite.tar.gz"
fi

tar -xzf demo-suite.tar.gz
chmod +x sds-agent sds-server scripts/*.sh

echo "=== 2. Setting Minimal Capabilities ==="
# Grant raw socket and process inspection capabilities
sudo setcap cap_net_raw,cap_dac_read_search,cap_sys_ptrace+ep ./sds-agent || true

echo "=== 3. Starting Services ==="
export SDS_TOKEN=$(openssl rand -hex 16)
mkdir -p data

# Start agent in background
sudo SDS_TOKEN=$SDS_TOKEN ./sds-agent serve &
AGENT_PID=$!

# Start server in background
SDS_TOKEN=$SDS_TOKEN SDS_AGENT_ADDR=http://127.0.0.1:7471 SDS_DB=data/eval.duckdb ./sds-server &
SERVER_PID=$!

trap "kill $AGENT_PID $SERVER_PID 2>/dev/null || true" EXIT

sleep 2

echo ""
echo "=========================================================="
echo " Cryptographic Discovery Suite is RUNNING!"
echo " URL: http://127.0.0.1:8080"
echo " SDS_TOKEN: $SDS_TOKEN"
echo "=========================================================="
echo ""
echo "To test TLS traffic, run in a separate terminal:"
echo "  cd ~/sds-eval && ./scripts/gen-traffic.sh 40"
echo ""

# Keep running for reviewer inspection
wait $SERVER_PID
```

---

## 3. Step-by-Step Feature Verification

### Test 1: Filesystem Discovery & Deduplication (30 seconds)
1. Open your browser and navigate to `http://127.0.0.1:8080/runs`.
2. Click **Run Filesystem Scan**.
3. A new run will appear in the table in `running` state and transition to **`partial`** within 2 seconds.
4. Click on the run to view the **Filesystem Discovery Report**:
   - **Metrics Bar:**
     - **Found:** 13
     - **Unique:** 8
     - **Duplicates:** 5
     - **Valid:** 11
     - **Expiring:** 1
     - **Expired:** 1
     - **Parse Failed:** 1
     - **Access Denied:** 1
   - **Coverage Banner:** Explains that `.test-certs/corpus/restricted_dir` returned `permission denied`.
   - **Charts:** Inspect the Validity Donut chart, Key Sizes distribution, and Top Issuers horizontal bar chart.
   - **Certificate Table:**
     - Type `expired` in the search/filter bar to locate the expired certificate.
     - Click any row to open the **Certificate Detail Drawer**, verifying the SHA-256 fingerprint, SANs, chain position, and owner confidence.

---

### Test 2: Passive TLS Capture & Attribution (45 seconds)
1. In a separate terminal, start the demo TLS traffic generator:
   ```bash
   ./scripts/gen-traffic.sh 30
   ```
   *This starts OpenSSL servers on port 8443 (TLS 1.2) and port 8444 (TLS 1.3), issuing continuous curl requests.*
2. Return to `http://127.0.0.1:8080/runs` and click **Run TLS Scan** (duration 30 seconds).
3. Once completed, click the run to open the **TLS Metadata Scan Report**:
   - **Metric Cards:**
     - **Events:** Total count of TLS record headers observed (e.g., 20+).
     - **Connections:** Distinct network flows.
     - **Unique Endpoints:** Observed `ip:port` destinations.
     - **Certs Observed:** Discovered via TLS 1.2 passive extraction and TLS 1.3 active probing.
   - **"How is this total computed?" Expander:** Click to view the exact mathematical formula (`SUM(record_count)`) and per-connection breakdown.
   - **Network Endpoints & Flows Table:**
     - Expand `127.0.0.1:8443` & `127.0.0.1:8444` rows.
     - Verify protocol versions (`TLS1.2` and `TLS1.3`), cipher suites, and process attribution (`curl`, `openssl`).
     - Notice the `observed` source badge on TLS 1.2 certificates and the `probed` badge on TLS 1.3 certificates.

---

### Test 3: Cryptographic Relationship Map (30 seconds)
1. On the TLS Scan Report, locate the **Cryptographic Relationship Map**.
2. Notice the left-to-right columnar layout:
   $$\text{Host} \longrightarrow \text{Process} \longrightarrow \text{Endpoint} \longrightarrow \text{Certificate}$$
3. Toggle between **This Run** and **Lifetime** scope in the map header.
4. Click on any node to view the property inspector side drawer.
5. Notice that expired or vulnerable certificates receive distinct amber/red warning indicators.

---

### Test 4: Automated Scan Scheduling
1. On the main `/runs` view, click **Schedules**.
2. Toggle the switch for **Filesystem Scan** or **TLS Scan**.
3. Select an interval of **30 seconds (demo)**.
4. Watch as scheduled runs automatically appear with `trigger = scheduled` upon each interval tick without interrupting manual operations.

---

## License & Copyright

Copyright (C) 2026 Alvin Alvo ([https://github.com/alvin-alvo/sec-discovery-suite](https://github.com/alvin-alvo/sec-discovery-suite)). All rights reserved.

Distributed under the GNU General Public License v3.0 (GPLv3). See [LICENSE](LICENSE) for details.

