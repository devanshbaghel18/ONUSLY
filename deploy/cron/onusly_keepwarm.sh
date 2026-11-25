#!/usr/bin/env bash
# ==============================================================================
# ONUSLY — 5-Minute EC2 Keep-Warm, Health Probe & Auto-Recovery Script
# Prevents cold-start latency across Go, DynamoDB, Nginx TLS, and optional edge
# endpoints, and recovers hung local services with anti-thrash cooldown.
# ==============================================================================

set -uo pipefail

ENV_FILE="${ONUSLY_ENV_FILE:-/etc/onusly/onusly.env}"
LOG_FILE="${KEEPWARM_LOG_FILE:-/var/log/onusly/keepwarm.log}"
LOCK_FILE="/run/onusly-keepwarm.lock"
GO_RESTART_STAMP="/run/onusly-keepwarm.last_restart"
NGINX_RESTART_STAMP="/run/onusly-nginx.last_restart"
COOLDOWN_SECONDS="${KEEPWARM_COOLDOWN_SECONDS:-600}"

if [ ! -w /run ] 2>/dev/null; then
  LOCK_FILE="/tmp/onusly-keepwarm.lock"
  GO_RESTART_STAMP="/tmp/onusly-keepwarm.last_restart"
  NGINX_RESTART_STAMP="/tmp/onusly-nginx.last_restart"
fi

# Prevent overlapping executions
exec 9>"$LOCK_FILE"
if ! flock -n 9; then
  exit 0
fi

# Load runtime config safely if present
if [ -r "$ENV_FILE" ]; then
  set -a
  # shellcheck source=/dev/null
  . "$ENV_FILE"
  set +a
fi

TARGET_HOST="${HOST:-127.0.0.1}"
if [ "$TARGET_HOST" = "localhost" ] || [ -z "$TARGET_HOST" ]; then
  TARGET_HOST="127.0.0.1"
fi
TARGET_PORT="${PORT:-8080}"
PUBLIC_URL="${KEEPWARM_PUBLIC_URL:-}"

mkdir -p "$(dirname "$LOG_FILE")" 2>/dev/null || true

log_line() {
  local level="$1"
  local msg="$2"
  local line
  line="$(date -u +"%Y-%m-%dT%H:%M:%SZ") [${level}] ${msg}"
  echo "$line"
  if [ -w "$(dirname "$LOG_FILE")" ] 2>/dev/null; then
    echo "$line" >> "$LOG_FILE"
  fi
}

can_restart() {
  local stamp_file="$1"
  local now
  now="$(date +%s)"
  if [ -f "$stamp_file" ]; then
    local last
    last="$(cat "$stamp_file" 2>/dev/null || echo 0)"
    if [ $((now - last)) -lt "$COOLDOWN_SECONDS" ]; then
      return 1
    fi
  fi
  return 0
}

mark_restarted() {
  local stamp_file="$1"
  date +%s > "$stamp_file" 2>/dev/null || true
}

probe_http() {
  local url="$1"
  local max_time="$2"
  shift 2
  curl -sS -o /dev/null -w "%{http_code} %{time_total}" --max-time "$max_time" "$@" "$url" 2>/dev/null || echo "000 0.000"
}

restart_service_guarded() {
  local svc="$1"
  local stamp_file="$2"
  if can_restart "$stamp_file"; then
    log_line "WARN" "Triggering guarded restart of ${svc}.service..."
    mark_restarted "$stamp_file"
    if systemctl restart "$svc"; then
      sleep 3
      return 0
    fi
    log_line "ERROR" "Failed to restart ${svc}.service"
    return 1
  else
    log_line "WARN" "Skipping ${svc}.service restart (cooldown ${COOLDOWN_SECONDS}s active)"
    return 1
  fi
}

EXIT_CODE=0

# 1. Ensure onusly.service is active
if ! systemctl is-active --quiet onusly 2>/dev/null; then
  log_line "WARN" "onusly.service is not active"
  restart_service_guarded "onusly" "$GO_RESTART_STAMP" || EXIT_CODE=1
fi

# 2. Probe local Go backend (/health + /health/warm)
read -r GO_CODE GO_TIME <<< "$(probe_http "http://${TARGET_HOST}:${TARGET_PORT}/health" 5)"
if [ "$GO_CODE" != "200" ]; then
  sleep 2
  read -r GO_CODE GO_TIME <<< "$(probe_http "http://${TARGET_HOST}:${TARGET_PORT}/health" 5)"
fi

if [ "$GO_CODE" != "200" ]; then
  log_line "ERROR" "Go /health probe failed after retry (http=${GO_CODE})"
  if restart_service_guarded "onusly" "$GO_RESTART_STAMP"; then
    read -r GO_CODE GO_TIME <<< "$(probe_http "http://${TARGET_HOST}:${TARGET_PORT}/health" 5)"
  fi
  if [ "$GO_CODE" != "200" ]; then
    log_line "ERROR" "Go backend remains unhealthy after recovery attempt (http=${GO_CODE})"
    journalctl -u onusly -n 20 --no-pager 2>/dev/null | while IFS= read -r jline; do
      log_line "DIAG" "onusly: ${jline}"
    done
    EXIT_CODE=1
  fi
fi

# 3. Warm DynamoDB connection pool & IMDS credentials via /health/warm
read -r WARM_CODE WARM_TIME <<< "$(probe_http "http://${TARGET_HOST}:${TARGET_PORT}/health/warm" 5)"
if [ "$WARM_CODE" != "200" ]; then
  log_line "WARN" "Go /health/warm returned http=${WARM_CODE} (${WARM_TIME}s)"
  if [ "$WARM_CODE" = "000" ]; then
    EXIT_CODE=1
  fi
fi

# 4. Ensure nginx.service is active and probe local TLS proxy
if ! systemctl is-active --quiet nginx 2>/dev/null; then
  log_line "WARN" "nginx.service is not active"
  restart_service_guarded "nginx" "$NGINX_RESTART_STAMP" || EXIT_CODE=1
fi

read -r NGX_CODE NGX_TIME <<< "$(probe_http "https://127.0.0.1/health" 5 -k -H "Host: api.onusly.com")"
if [ "$NGX_CODE" != "200" ]; then
  sleep 2
  read -r NGX_CODE NGX_TIME <<< "$(probe_http "https://127.0.0.1/health" 5 -k -H "Host: api.onusly.com")"
fi

if [ "$NGX_CODE" != "200" ]; then
  log_line "ERROR" "Nginx HTTPS proxy probe failed after retry (http=${NGX_CODE})"
  if restart_service_guarded "nginx" "$NGINX_RESTART_STAMP"; then
    read -r NGX_CODE NGX_TIME <<< "$(probe_http "https://127.0.0.1/health" 5 -k -H "Host: api.onusly.com")"
  fi
  if [ "$NGX_CODE" != "200" ]; then
    log_line "ERROR" "Nginx HTTPS proxy remains unhealthy (http=${NGX_CODE})"
    EXIT_CODE=1
  fi
fi

# 5. Optional public edge keep-warm probe (e.g. KEEPWARM_PUBLIC_URL=https://api.onusly.com/health)
PUBLIC_STATUS="skipped"
if [ -n "$PUBLIC_URL" ]; then
  read -r PUB_CODE PUB_TIME <<< "$(probe_http "$PUBLIC_URL" 8)"
  PUBLIC_STATUS="http=${PUB_CODE}(${PUB_TIME}s)"
  if [ "$PUB_CODE" != "200" ]; then
    log_line "WARN" "Public edge probe ${PUBLIC_URL} returned ${PUBLIC_STATUS}"
  fi
fi

log_line "INFO" "keepwarm go_health=${GO_CODE}(${GO_TIME}s) go_warm=${WARM_CODE}(${WARM_TIME}s) nginx_tls=${NGX_CODE}(${NGX_TIME}s) public=${PUBLIC_STATUS}"
exit "$EXIT_CODE"
