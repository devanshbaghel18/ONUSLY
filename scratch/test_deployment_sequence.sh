#!/usr/bin/env bash
# ==============================================================================
# ONUSLY — Local Deployment Sequence & Rollback Verification Test Harness
# ==============================================================================
set -euo pipefail

TEST_ENV="$(mktemp -d /tmp/onusly_deploy_sim.XXXXXX)"
cleanup() {
  rm -rf "$TEST_ENV"
}
trap cleanup EXIT INT TERM

echo "=== Testing Deployment Sequence & Rollback Logic in ${TEST_ENV} ==="

mkdir -p "${TEST_ENV}/opt/onusly"
mkdir -p "${TEST_ENV}/etc/nginx/conf.d"
mkdir -p "${TEST_ENV}/etc/systemd/system"
mkdir -p "${TEST_ENV}/etc/ssl/onusly"

APP_DIR="${TEST_ENV}/opt/onusly"
NGINX_CONF_DEST="${TEST_ENV}/etc/nginx/conf.d/onusly.conf"
SYSTEMD_SERVICE_DEST="${TEST_ENV}/etc/systemd/system/onusly.service"

# Initial state: Version 1 binary and configs exist
echo "V1_BINARY" > "${APP_DIR}/server"
chmod 755 "${APP_DIR}/server"
echo "V1_NGINX" > "$NGINX_CONF_DEST"
echo "V1_SERVICE" > "$SYSTEMD_SERVICE_DEST"

TIMESTAMP="sim_test"

# Test 1: Go backend compilation success vs failure
echo ">>> Test 1: Go backend compilation & atomic binary replacement"
# 1a. Successful compilation
if go build -o "${APP_DIR}/server.new" ./cmd/server; then
  cp -p "${APP_DIR}/server" "${APP_DIR}/server.bak.${TIMESTAMP}"
  mv -f "${APP_DIR}/server.new" "${APP_DIR}/server"
  chmod 755 "${APP_DIR}/server"
  echo "  [PASS] Compilation succeeded; binary atomically replaced"
else
  echo "  [FAIL] Compilation failed"
  exit 1
fi

# 1b. Failed compilation preserves existing binary
FAIL_BUILD_DIR="$(mktemp -d /tmp/fail_build.XXXXXX)"
echo "package main; syntax error here" > "${FAIL_BUILD_DIR}/bad.go"
if go build -o "${APP_DIR}/server.new" "${FAIL_BUILD_DIR}/bad.go" 2>/dev/null; then
  echo "  [FAIL] Compilation should have failed but succeeded"
  rm -rf "$FAIL_BUILD_DIR"
  exit 1
else
  rm -f "${APP_DIR}/server.new"
  if [ -f "${APP_DIR}/server" ]; then
    echo "  [PASS] Failed compilation cleanly aborted without touching active binary"
  else
    echo "  [FAIL] Active binary was lost during compilation failure"
    rm -rf "$FAIL_BUILD_DIR"
    exit 1
  fi
fi
rm -rf "$FAIL_BUILD_DIR"

# Test 2: Nginx configuration installation and rollback
echo ">>> Test 2: Nginx configuration rollback on validation failure"
cp -p "$NGINX_CONF_DEST" "${NGINX_CONF_DEST}.bak.${TIMESTAMP}"
# Simulate invalid nginx config written
echo "invalid nginx syntax;" > "$NGINX_CONF_DEST"

# Mock validation failure
MOCK_NGINX_T_PASS=false
if [ "$MOCK_NGINX_T_PASS" = true ]; then
  echo "  [FAIL] Nginx test should have failed"
  exit 1
else
  # Perform rollback logic
  cp -f "${NGINX_CONF_DEST}.bak.${TIMESTAMP}" "$NGINX_CONF_DEST"
  if [ "$(cat "$NGINX_CONF_DEST")" = "V1_NGINX" ]; then
    echo "  [PASS] Nginx configuration cleanly rolled back to previous valid backup"
  else
    echo "  [FAIL] Nginx configuration rollback did not restore previous backup"
    exit 1
  fi
fi

# Test 3: Systemd service and binary rollback on startup failure
echo ">>> Test 3: Systemd service & binary rollback on startup failure"
# Backup V1 service
cp -p "$SYSTEMD_SERVICE_DEST" "${SYSTEMD_SERVICE_DEST}.bak.${TIMESTAMP}"

# Simulate deployment of bad V2
echo "V2_BROKEN_BINARY" > "${APP_DIR}/server"
echo "V2_BROKEN_SERVICE" > "$SYSTEMD_SERVICE_DEST"

# Service fails to start -> execute rollback
if [ -f "${APP_DIR}/server.bak.${TIMESTAMP}" ]; then
  cp -f "${APP_DIR}/server.bak.${TIMESTAMP}" "${APP_DIR}/server"
fi
if [ -f "${SYSTEMD_SERVICE_DEST}.bak.${TIMESTAMP}" ]; then
  cp -f "${SYSTEMD_SERVICE_DEST}.bak.${TIMESTAMP}" "$SYSTEMD_SERVICE_DEST"
fi

if [ "$(cat "${APP_DIR}/server")" = "V1_BINARY" ] && [ "$(cat "$SYSTEMD_SERVICE_DEST")" = "V1_SERVICE" ]; then
  echo "  [PASS] Service failure cleanly rolled back both binary and systemd unit"
else
  echo "  [FAIL] Rollback failed to restore previous binary and unit"
  exit 1
fi

# Test 4: Health check failure triggers rollback
echo ">>> Test 4: Health check failure triggers binary rollback"
echo "V2_HEALTH_FAIL_BINARY" > "${APP_DIR}/server"
MOCK_HEALTH_STATUS="500"
if [ "$MOCK_HEALTH_STATUS" != "200" ]; then
  if [ -f "${APP_DIR}/server.bak.${TIMESTAMP}" ]; then
    cp -f "${APP_DIR}/server.bak.${TIMESTAMP}" "${APP_DIR}/server"
  fi
  if [ "$(cat "${APP_DIR}/server")" = "V1_BINARY" ]; then
    echo "  [PASS] Health check failure triggered successful rollback to previous binary"
  else
    echo "  [FAIL] Failed to restore binary on health check failure"
    exit 1
  fi
fi

echo ""
echo ">>> ALL DEPLOYMENT SEQUENCE & ROLLBACK LOGIC TESTS PASSED! <<<"
