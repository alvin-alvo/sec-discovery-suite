#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 2 ]; then
  echo "usage: $0 <binary> <max_bytes>" >&2
  exit 1
fi

BIN="$1"
MAX_BYTES="$2"

if [ ! -f "$BIN" ]; then
  echo "error: file not found: $BIN" >&2
  exit 1
fi

SIZE=$(stat -c %s "$BIN")
echo "$BIN size: $SIZE bytes (max allowed: $MAX_BYTES bytes)"

if [ "$SIZE" -gt "$MAX_BYTES" ]; then
  echo "FAIL: $BIN ($SIZE bytes) exceeds maximum ($MAX_BYTES bytes)"
  exit 1
else
  echo "PASS: $BIN ($SIZE bytes) is within budget ($MAX_BYTES bytes)"
  exit 0
fi
