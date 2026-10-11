#!/usr/bin/env bash
set -euo pipefail

DIR="${1:-.test-certs}"
mkdir -p "$DIR"

# (a) Self-signed RSA-2048 cert as PEM
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$DIR/rsa2048.key" \
  -out "$DIR/rsa2048.pem" \
  -days 365 -subj "/CN=test-rsa2048" 2>/dev/null

# (b) Self-signed EC P-256 cert as PEM
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes \
  -keyout "$DIR/ecp256.key" \
  -out "$DIR/ecp256.pem" \
  -days 365 -subj "/CN=test-ecp256" 2>/dev/null

# (c) PEM containing a certificate AND its PRIVATE KEY block
cat "$DIR/rsa2048.key" "$DIR/rsa2048.pem" > "$DIR/combined.pem"

echo "Test certificates generated in $DIR"
