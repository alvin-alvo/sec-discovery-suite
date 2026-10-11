#!/usr/bin/env bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$DIR"

DURATION=${1:-40}
CERT=".test-certs/corpus/cert_and_key.pem"

if [ ! -f "$CERT" ]; then
    echo "Cert file $CERT not found, running seed..."
    make seed
fi

# Clean up any previous listeners on test ports
fuser -k 8443/tcp 8444/tcp 2>/dev/null || true

echo "Starting openssl s_server on 8443 (TLS 1.2)..."
openssl s_server -accept 8443 -cert "$CERT" -key "$CERT" -tls1_2 -www >/dev/null 2>&1 &
PID_12=$!

echo "Starting openssl s_server on 8444 (TLS 1.3)..."
openssl s_server -accept 8444 -cert "$CERT" -key "$CERT" -tls1_3 -www >/dev/null 2>&1 &
PID_13=$!

cleanup() {
    echo "Stopping background servers..."
    kill "$PID_12" "$PID_13" 2>/dev/null || true
    fuser -k 8443/tcp 8444/tcp 2>/dev/null || true
}
trap cleanup EXIT INT TERM

sleep 1
echo "Generating TLS traffic for $DURATION seconds..."

END_TIME=$((SECONDS + DURATION))
while [ $SECONDS -lt $END_TIME ]; do
    # TLS 1.2 local loopback
    curl -sk --tlsv1.2 --tls-max 1.2 https://localhost:8443/ >/dev/null 2>&1 || true

    # TLS 1.3 local loopback
    curl -sk --tlsv1.3 --tls-max 1.3 https://localhost:8444/ >/dev/null 2>&1 || true

    # Best-effort outbound curls
    curl -sk --max-time 1 https://example.com/ >/dev/null 2>&1 || true
    curl -sk --max-time 1 https://www.google.com/ >/dev/null 2>&1 || true

    sleep 1
done

echo "Traffic generation finished."
