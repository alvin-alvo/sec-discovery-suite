#!/usr/bin/env bash
set -e

TARGET=${1:-bin/sds-agent}

if [ ! -f "$TARGET" ]; then
    echo "Binary $TARGET not found. Run make agent first."
    exit 1
fi

echo "Setting capabilities on $TARGET..."
sudo setcap cap_net_raw,cap_dac_read_search,cap_sys_ptrace+ep "$TARGET"
echo "Capabilities set successfully:"
getcap "$TARGET"
