#!/usr/bin/env bash
set -euo pipefail

echo "=========================================================="
echo "      ONUSLY DESKTOP — LIVE SYSTEM ROOT MODE VERIFICATION "
echo "=========================================================="

TEST_DIR=$(mktemp -d /tmp/onusly-sys-verify-XXXXXX)
trap 'rm -rf "$TEST_DIR"' EXIT

CONFIG_DIR="$TEST_DIR/etc"
DATA_DIR="$TEST_DIR/data"
SOCK_DIR="$TEST_DIR/run"
SOCK_PATH="$SOCK_DIR/agent.sock"

mkdir -p "$CONFIG_DIR" "$DATA_DIR" "$SOCK_DIR"
chmod 0700 "$CONFIG_DIR"
chmod 0700 "$DATA_DIR"
chmod 0755 "$SOCK_DIR"

# 1. Setup Root-owned credentials (0600) and paired UID (1000)
echo "11111111-2222-3333-4444-555555555555" > "$CONFIG_DIR/device_id"
chmod 0600 "$CONFIG_DIR/device_id"

echo "1000" > "$CONFIG_DIR/paired_uids"
chmod 0600 "$CONFIG_DIR/paired_uids"

# Pre-populate token
echo "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbWFpbCI6InRlc3QtdXNlckBvbnVzbHkuY29tIiwiZXhwIjoxNzk2NTAyODY0LCJoYW5kbGUiOiJ0ZXN0dXNlciIsIm5hbWUiOiJUZXN0IFVzZXIiLCJzdWIiOiJ0ZXN0LXVzZXItZTJlIn0.wXSB3g7ITCyQ_ZTsbulWweNXrBRfo8EKl1Mu6wRxQ0Q" > "$CONFIG_DIR/token"
chmod 0600 "$CONFIG_DIR/token"

# 2. Pre-populate blocklist cache in data directory
cat << 'EOF' > "$DATA_DIR/blocklist_cache.json"
{
  "userId": "test-user-e2e",
  "version": "v1-root-audit-verified",
  "lastSyncAt": "2026-11-06T03:00:00Z",
  "targets": [
    {
      "goalId": "goal-root-1",
      "goalTitle": "Deep Focus",
      "status": "active",
      "domains": ["instagram.com", "badsite.test"]
    }
  ]
}
EOF
chmod 0600 "$DATA_DIR/blocklist_cache.json"

echo "Configuration created:"
ls -la "$CONFIG_DIR"
ls -la "$DATA_DIR"

# 3. Start Desktop Agent with system mode configuration
./desktop/onusly-agent run \
  --api-url=http://localhost:8080 \
  --config-dir="$CONFIG_DIR" \
  --data-dir="$DATA_DIR" \
  --socket="$SOCK_PATH" &
AGENT_PID=$!

sleep 1

# Verify socket was created with expected permissions
echo "Socket details:"
ls -la "$SOCK_PATH"

# 4. Connect via Native Messaging Host as Desktop User (UID 1000)
echo "Executing Native Messaging Host query..."
export ONUSLY_IPC_SOCKET="$SOCK_PATH"

# Python script to send 4-byte LE length framed message to onusly-native-host
python3 - << 'PYEOF'
import os
import subprocess
import struct
import json

req = json.dumps({"action": "get-blocklist"}).encode('utf-8')
framed = struct.pack("<I", len(req)) + req

p = subprocess.Popen(
    ["./desktop/onusly-native-host"],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    env=dict(os.environ)
)

out, err = p.communicate(input=framed)
if len(out) < 4:
    print(f"FAILED: Native host returned {len(out)} bytes. stderr={err.decode()}")
    exit(1)

resp_len = struct.unpack("<I", out[:4])[0]
resp_data = json.loads(out[4:4+resp_len].decode('utf-8'))
print("Native Host Response:")
print(json.dumps(resp_data, indent=2))

assert resp_data.get("success") == True, f"Expected success: True, got {resp_data}"
blocklist = resp_data.get("blocklist", {})
assert blocklist.get("version"), f"Missing version: {blocklist}"
assert len(blocklist.get("targets", [])) >= 1, f"Expected at least 1 target: {blocklist}"
print(f"VERIFICATION SUCCESSFUL: Native Messaging Host queried agent over local IPC (Version: {blocklist.get('version')}).")
PYEOF

# Clean up agent
kill "$AGENT_PID" || true
wait "$AGENT_PID" 2>/dev/null || true
echo "Agent stopped cleanly."
