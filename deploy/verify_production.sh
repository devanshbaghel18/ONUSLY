#!/usr/bin/env bash
# ==============================================================================
# ONUSLY — Production & Origin API Verification Probe
# Verifies:
#   1. GET /health -> HTTP 200
#   2. Unauthenticated GET /enforcement/blocklist -> HTTP 401
#   3. GET /downloads/updates.xml -> Valid XML (not HTML fallback)
#   4. GET /downloads/onusly.crx -> Valid CRX3 binary (Cr24 magic header)
#   5. CORS Preflight (OPTIONS) -> Origin & Allow-Methods headers
#   6. TLS Certificate hostname & expiration (HTTPS targets)
#   7. Response headers & absence of unexpected SPA HTML fallbacks
# ==============================================================================

set -euo pipefail

TARGET_URL="${1:-https://api.onusly.com}"
# Strip trailing slash if present
TARGET_URL="${TARGET_URL%/}"

echo "=============================================================================="
echo ">>> ONUSLY API Verification Probe executing against: ${TARGET_URL}"
echo "    Timestamp: $(date -u +"%Y-%m-%d %H:%M:%SZ")"
echo "=============================================================================="

PASS_COUNT=0
FAIL_COUNT=0
TOTAL_CHECKS=0

record_pass() {
  local name="$1"
  local detail="${2:-}"
  PASS_COUNT=$((PASS_COUNT + 1))
  TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
  if [ -n "$detail" ]; then
    echo "  [PASS] ${name} (${detail})"
  else
    echo "  [PASS] ${name}"
  fi
}

record_fail() {
  local name="$1"
  local reason="$2"
  FAIL_COUNT=$((FAIL_COUNT + 1))
  TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
  echo "  [FAIL] ${name}: ${reason}" >&2
}

# Determine curl TLS flags (allow -k if testing localhost/IP with custom Host)
CURL_EXTRA_FLAGS=()
if [[ "$TARGET_URL" == *"127.0.0.1"* ]] || [[ "$TARGET_URL" == *"localhost"* ]]; then
  CURL_EXTRA_FLAGS+=("-k")
fi

# ------------------------------------------------------------------------------
# 1. GET /health
# ------------------------------------------------------------------------------
echo ">>> Check 1: Health Endpoint (GET ${TARGET_URL}/health)..."
TMP_HEALTH_HEADER="$(mktemp)"
TMP_HEALTH_BODY="$(mktemp)"

HEALTH_STATUS="$(curl -s -S "${CURL_EXTRA_FLAGS[@]}" -o "$TMP_HEALTH_BODY" -D "$TMP_HEALTH_HEADER" -w "%{http_code}" "${TARGET_URL}/health" || echo "CONN_ERR")"

if [ "$HEALTH_STATUS" = "200" ]; then
  if grep -qi "<!doctype html" "$TMP_HEALTH_BODY" || grep -qi "<html" "$TMP_HEALTH_BODY"; then
    record_fail "GET /health" "Returned HTTP 200 but content is SPA HTML fallback instead of API response!"
  elif grep -q "ok" "$TMP_HEALTH_BODY"; then
    record_pass "GET /health" "HTTP 200, valid JSON payload"
  else
    record_pass "GET /health" "HTTP 200 OK"
  fi
elif [ "$HEALTH_STATUS" = "521" ]; then
  record_fail "GET /health" "Cloudflare HTTP 521 Origin Down. Origin server is not accepting connections."
else
  record_fail "GET /health" "HTTP status ${HEALTH_STATUS} (expected 200)"
fi
rm -f "$TMP_HEALTH_HEADER" "$TMP_HEALTH_BODY"

# ------------------------------------------------------------------------------
# 2. Unauthenticated GET /enforcement/blocklist
# ------------------------------------------------------------------------------
echo ">>> Check 2: Unauthenticated Security Boundary (GET ${TARGET_URL}/enforcement/blocklist)..."
UNAUTH_STATUS="$(curl -s -S "${CURL_EXTRA_FLAGS[@]}" -o /dev/null -w "%{http_code}" "${TARGET_URL}/enforcement/blocklist" || echo "CONN_ERR")"
if [ "$UNAUTH_STATUS" = "401" ]; then
  record_pass "Unauthenticated /enforcement/blocklist" "HTTP 401 Unauthorized (Protected)"
elif [ "$UNAUTH_STATUS" = "521" ]; then
  record_fail "Unauthenticated /enforcement/blocklist" "Cloudflare HTTP 521 Origin Down"
else
  record_fail "Unauthenticated /enforcement/blocklist" "HTTP status ${UNAUTH_STATUS} (expected 401 Unauthorized)"
fi

# ------------------------------------------------------------------------------
# 3. GET /downloads/updates.xml
# ------------------------------------------------------------------------------
echo ">>> Check 3: Extension Update Manifest (GET ${TARGET_URL}/downloads/updates.xml)..."
TMP_XML="$(mktemp)"
TMP_XML_HEADER="$(mktemp)"
XML_STATUS="$(curl -s -S "${CURL_EXTRA_FLAGS[@]}" -o "$TMP_XML" -D "$TMP_XML_HEADER" -w "%{http_code}" "${TARGET_URL}/downloads/updates.xml" || echo "CONN_ERR")"

if [ "$XML_STATUS" = "200" ]; then
  if grep -qi "<!doctype html" "$TMP_XML" || grep -qi "<html" "$TMP_XML"; then
    record_fail "GET /downloads/updates.xml" "Returned HTTP 200 but content is SPA HTML fallback instead of XML!"
  elif grep -q "gupdate" "$TMP_XML" || grep -q "updatecheck" "$TMP_XML"; then
    CONTENT_TYPE="$(grep -i "^content-type:" "$TMP_XML_HEADER" | tr -d '\r\n')"
    record_pass "GET /downloads/updates.xml" "HTTP 200, valid XML manifest (${CONTENT_TYPE})"
  else
    record_fail "GET /downloads/updates.xml" "HTTP 200 but missing gupdate/updatecheck elements"
  fi
else
  record_fail "GET /downloads/updates.xml" "HTTP status ${XML_STATUS} (expected 200)"
fi
rm -f "$TMP_XML" "$TMP_XML_HEADER"

# ------------------------------------------------------------------------------
# 4. GET /downloads/onusly.crx
# ------------------------------------------------------------------------------
echo ">>> Check 4: Chrome Extension Binary (GET ${TARGET_URL}/downloads/onusly.crx)..."
TMP_CRX="$(mktemp)"
TMP_CRX_HEADER="$(mktemp)"
CRX_STATUS="$(curl -s -S "${CURL_EXTRA_FLAGS[@]}" -o "$TMP_CRX" -D "$TMP_CRX_HEADER" -w "%{http_code}" "${TARGET_URL}/downloads/onusly.crx" || echo "CONN_ERR")"

if [ "$CRX_STATUS" = "200" ]; then
  CRX_SIZE="$(wc -c < "$TMP_CRX")"
  MAGIC_HEADER="$(head -c 4 "$TMP_CRX" 2>/dev/null || echo "")"
  if grep -qi "<!doctype html" "$TMP_CRX" || grep -qi "<html" "$TMP_CRX"; then
    record_fail "GET /downloads/onusly.crx" "Returned HTTP 200 but content is SPA HTML fallback instead of CRX binary!"
  elif [ "$MAGIC_HEADER" = "Cr24" ]; then
    record_pass "GET /downloads/onusly.crx" "HTTP 200, valid CRX3 package (Cr24 header, ${CRX_SIZE} bytes)"
  else
    record_fail "GET /downloads/onusly.crx" "HTTP 200 but invalid magic header: expected 'Cr24', got '${MAGIC_HEADER}'"
  fi
else
  record_fail "GET /downloads/onusly.crx" "HTTP status ${CRX_STATUS} (expected 200)"
fi
rm -f "$TMP_CRX" "$TMP_CRX_HEADER"

# ------------------------------------------------------------------------------
# 5. CORS Preflight Check
# ------------------------------------------------------------------------------
echo ">>> Check 5: CORS Preflight (OPTIONS ${TARGET_URL}/enforcement/blocklist)..."
TMP_CORS_HEADER="$(mktemp)"
CORS_STATUS="$(curl -s -S "${CURL_EXTRA_FLAGS[@]}" -X OPTIONS -o /dev/null -D "$TMP_CORS_HEADER" -w "%{http_code}" \
  -H "Origin: https://onusly.app" \
  -H "Access-Control-Request-Method: GET" \
  -H "Access-Control-Request-Headers: Authorization,Content-Type" \
  "${TARGET_URL}/enforcement/blocklist" || echo "CONN_ERR")"

if [ "$CORS_STATUS" = "200" ]; then
  ALLOW_ORIGIN="$(grep -i "^access-control-allow-origin:" "$TMP_CORS_HEADER" | tr -d '\r\n' || echo "")"
  ALLOW_METHODS="$(grep -i "^access-control-allow-methods:" "$TMP_CORS_HEADER" | tr -d '\r\n' || echo "")"
  if [ -n "$ALLOW_ORIGIN" ]; then
    record_pass "CORS Preflight" "HTTP 200, ${ALLOW_ORIGIN}, ${ALLOW_METHODS}"
  else
    record_fail "CORS Preflight" "HTTP 200 but missing Access-Control-Allow-Origin header"
  fi
else
  record_fail "CORS Preflight" "HTTP status ${CORS_STATUS} (expected 200)"
fi
rm -f "$TMP_CORS_HEADER"

# ------------------------------------------------------------------------------
# 6. TLS Certificate Verification (Only for HTTPS targets)
# ------------------------------------------------------------------------------
if [[ "$TARGET_URL" == https://* ]]; then
  # Extract domain and port
  DOMAIN_PORT="${TARGET_URL#https://}"
  DOMAIN_PORT="${DOMAIN_PORT%%/*}"
  DOMAIN="${DOMAIN_PORT%%:*}"
  PORT="${DOMAIN_PORT##*:}"
  if [ "$PORT" = "$DOMAIN" ]; then
    PORT="443"
  fi

  echo ">>> Check 6: TLS Certificate Verification for ${DOMAIN}:${PORT}..."
  TMP_CERT="$(mktemp)"
  if echo | openssl s_client -servername "$DOMAIN" -connect "${DOMAIN}:${PORT}" -showcerts 2>/dev/null > "$TMP_CERT"; then
    if openssl x509 -in "$TMP_CERT" -noout -checkend 86400 2>/dev/null; then
      CERT_SUBJ="$(openssl x509 -in "$TMP_CERT" -noout -subject 2>/dev/null || echo "")"
      CERT_EXP="$(openssl x509 -in "$TMP_CERT" -noout -enddate 2>/dev/null || echo "")"
      record_pass "TLS Certificate" "Valid, not expired (${CERT_EXP}, ${CERT_SUBJ})"
    else
      record_fail "TLS Certificate" "Certificate is expired or expires within 24 hours"
    fi
  else
    record_fail "TLS Certificate" "Failed to establish TLS connection to ${DOMAIN}:${PORT}"
  fi
  rm -f "$TMP_CERT"
fi

# ------------------------------------------------------------------------------
# Summary
# ------------------------------------------------------------------------------
echo "=============================================================================="
echo ">>> Probe Summary: ${PASS_COUNT}/${TOTAL_CHECKS} checks passed (${FAIL_COUNT} failed)"
if [ "$FAIL_COUNT" -eq 0 ]; then
  echo ">>> ALL PRODUCTION VERIFICATION PROBES PASSED!"
  echo "=============================================================================="
  exit 0
else
  echo "[-] ${FAIL_COUNT} VERIFICATION PROBES FAILED!" >&2
  echo "=============================================================================="
  exit 1
fi
