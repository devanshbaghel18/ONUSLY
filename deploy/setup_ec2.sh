#!/usr/bin/env bash
# ==============================================================================
# ONUSLY — Production EC2 Deployment & Hardening Script (Amazon Linux 2023)
# Target Architecture:
#   Client -> HTTPS -> Cloudflare (Full Strict) -> EC2 Nginx :443 -> Go Backend :8080
# Region: eu-north-1 (Stockholm)
# ==============================================================================

set -euo pipefail

TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
TARGET_REGION="eu-north-1"
ACTUAL_ACCOUNT_ID="631690098190"
APP_DIR="/opt/onusly"
ENV_FILE="/etc/onusly/onusly.env"
NGINX_CONF_DEST="/etc/nginx/conf.d/onusly.conf"
SYSTEMD_SERVICE_DEST="/etc/systemd/system/onusly.service"
SSL_DIR="/etc/ssl/onusly"
SSL_CERT_PATH="${SSL_DIR}/api.onusly.com.pem"
SSL_KEY_PATH="${SSL_DIR}/api.onusly.com.key"

DEV_MODE=false
TARGET_COMMIT=""

# Parse command line flags
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dev)
      DEV_MODE=true
      shift
      ;;
    --commit)
      TARGET_COMMIT="$2"
      shift 2
      ;;
    -h|--help)
      echo "Usage: sudo ./deploy/setup_ec2.sh [--commit <GIT_COMMIT_SHA>] [--dev]"
      echo "  --commit <SHA> : Deploy a specific reviewed git commit deterministically"
      echo "  --dev          : Allow bootstrap self-signed certificates (FOR NON-PROD DEVELOPMENT ONLY)"
      exit 0
      ;;
    *)
      echo "[-] Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

echo "=============================================================================="
echo ">>> ONUSLY Production Deployment & Hardening Execution: ${TIMESTAMP}"
echo "    Mode: $([ "$DEV_MODE" = true ] && echo "DEVELOPMENT (--dev)" || echo "PRODUCTION (Strict)")"
if [ -n "$TARGET_COMMIT" ]; then
  echo "    Target Commit: ${TARGET_COMMIT}"
fi
echo "=============================================================================="

# ------------------------------------------------------------------------------
# 1. Privilege Verification
# ------------------------------------------------------------------------------
if [ "$(id -u)" -ne 0 ]; then
  echo "[-] ERROR: This script must be run as root or with sudo privileges." >&2
  exit 1
fi
echo "[+] Privilege check passed: running as root."

# ------------------------------------------------------------------------------
# 2. Operating System Validation (Amazon Linux 2023)
# ------------------------------------------------------------------------------
if [ -f /etc/os-release ]; then
  # shellcheck source=/dev/null
  . /etc/os-release
  echo "[+] Detected OS: ${NAME:-Linux} ${VERSION_ID:-}"
  if [[ "${ID:-}" != "amzn" || "${VERSION_ID:-}" != "2023"* ]]; then
    echo "[!] WARNING: Host OS is not Amazon Linux 2023 (Detected: ${ID:-} ${VERSION_ID:-})."
    if [ "$DEV_MODE" = false ]; then
      echo "[-] Production deployment requires Amazon Linux 2023. Aborting." >&2
      exit 1
    fi
  fi
fi

# ------------------------------------------------------------------------------
# 3. Host Resource Verification (Disk & Memory)
# ------------------------------------------------------------------------------
echo ">>> Checking available system resources..."
AVAILABLE_DISK_MB="$(df -m "$APP_DIR" 2>/dev/null | awk 'NR==2 {print $4}' || df -m / | awk 'NR==2 {print $4}')"
if [ -n "$AVAILABLE_DISK_MB" ] && [ "$AVAILABLE_DISK_MB" -lt 500 ]; then
  echo "[-] ERROR: Insufficient disk space on ${APP_DIR} (${AVAILABLE_DISK_MB} MB free, minimum 500 MB required)." >&2
  exit 1
fi
echo "[+] Disk space verified: ${AVAILABLE_DISK_MB} MB available."

AVAILABLE_MEM_MB="$(free -m | awk '/^Mem:/ {print $7}')"
if [ -n "$AVAILABLE_MEM_MB" ] && [ "$AVAILABLE_MEM_MB" -lt 128 ]; then
  echo "[-] ERROR: Insufficient free memory (${AVAILABLE_MEM_MB} MB available, minimum 128 MB required)." >&2
  exit 1
fi
echo "[+] Memory verified: ${AVAILABLE_MEM_MB} MB available."

# ------------------------------------------------------------------------------
# 4. Required Package Dependencies
# ------------------------------------------------------------------------------
echo ">>> Checking required package dependencies..."
MISSING_PKGS=()
for pkg in git golang nginx openssl curl; do
  if ! command -v "$pkg" >/dev/null 2>&1; then
    MISSING_PKGS+=("$pkg")
  fi
done

if [ ${#MISSING_PKGS[@]} -gt 0 ]; then
  echo "[+] Installing missing packages: ${MISSING_PKGS[*]}"
  dnf install -y "${MISSING_PKGS[@]}"
else
  echo "[+] All required packages present (git, golang, nginx, openssl, curl)."
fi

# ------------------------------------------------------------------------------
# 5. Deployment Region Validation (eu-north-1 Stockholm)
# ------------------------------------------------------------------------------
echo ">>> Validating deployment region (${TARGET_REGION})..."
DETECTED_REGION=""
IMDS_TOKEN="$(curl -s -S -m 2 -X PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 60" 2>/dev/null || true)"
if [ -n "$IMDS_TOKEN" ]; then
  DETECTED_REGION="$(curl -s -S -m 2 -H "X-aws-ec2-metadata-token: $IMDS_TOKEN" http://169.254.169.254/latest/meta-data/placement/region 2>/dev/null || true)"
fi

if [ -n "$DETECTED_REGION" ]; then
  echo "[+] EC2 IMDSv2 confirmed region: ${DETECTED_REGION}"
  if [ "$DETECTED_REGION" != "$TARGET_REGION" ]; then
    echo "[-] ERROR: EC2 instance region is '${DETECTED_REGION}', but canonical database is in '${TARGET_REGION}'!" >&2
    exit 1
  fi
else
  echo "[!] Notice: IMDSv2 unreachable. Proceeding with target region: ${TARGET_REGION}."
fi

# ------------------------------------------------------------------------------
# 6. IAM Instance Profile & Identity Verification
# ------------------------------------------------------------------------------
echo ">>> Checking AWS IAM instance role integration..."
if [ -n "$IMDS_TOKEN" ]; then
  IAM_INFO="$(curl -s -S -m 2 -H "X-aws-ec2-metadata-token: $IMDS_TOKEN" http://169.254.169.254/latest/meta-data/iam/info 2>/dev/null || true)"
  if echo "$IAM_INFO" | grep -q "InstanceProfileArn"; then
    IAM_ROLE_NAME="$(echo "$IAM_INFO" | grep "InstanceProfileArn" | cut -d'"' -f4)"
    echo "[+] Attached EC2 IAM Instance Profile: ${IAM_ROLE_NAME}"
  else
    echo "[-] ERROR: No IAM instance profile attached to this EC2 instance!" >&2
    echo "    Backend DynamoDB calls will fail. Attach 'onusly-ec2-backend-role' via AWS EC2 console." >&2
    if [ "$DEV_MODE" = false ]; then
      exit 1
    fi
  fi
fi

if command -v aws >/dev/null 2>&1; then
  CALLER_ARN="$(aws sts get-caller-identity --query 'Arn' --output text 2>/dev/null || echo "UNAVAILABLE")"
  echo "[+] AWS STS Caller Identity: ${CALLER_ARN}"
fi

# ------------------------------------------------------------------------------
# 7. Environment Configuration Validation (/etc/onusly/onusly.env)
# ------------------------------------------------------------------------------
echo ">>> Checking environment configuration (${ENV_FILE})..."
mkdir -p /etc/onusly

if [ -f "$ENV_FILE" ]; then
  cp -p "$ENV_FILE" "${ENV_FILE}.bak.${TIMESTAMP}"
  chmod 600 "${ENV_FILE}.bak.${TIMESTAMP}"
  chown root:root "${ENV_FILE}.bak.${TIMESTAMP}" 2>/dev/null || true
  echo "[+] Backed up existing environment configuration to ${ENV_FILE}.bak.${TIMESTAMP}."
fi

if [ ! -f "$ENV_FILE" ]; then
  echo "[+] Creating initial environment template at ${ENV_FILE}..."
  cat << EOF > "$ENV_FILE"
HOST=127.0.0.1
PORT=8080
AWS_REGION=${TARGET_REGION}
DYNAMODB_TABLE=Onusly
# Production secrets (Required before service will start):
# GOOGLE_CLIENT_ID=
# GOOGLE_CLIENT_SECRET=
# JWT_SECRET=
EOF
  chmod 600 "$ENV_FILE"
  chown root:root "$ENV_FILE" 2>/dev/null || true
  echo "[-] ERROR: Template created at ${ENV_FILE}. Populate production secrets (JWT_SECRET) and rerun." >&2
  exit 1
fi

chmod 600 "$ENV_FILE"
chown root:root "$ENV_FILE" 2>/dev/null || true

# Security check: Ensure NO hardcoded AWS credentials in environment file
if grep -E -q "AWS_ACCESS_KEY_ID|AWS_SECRET_ACCESS_KEY" "$ENV_FILE"; then
  echo "[-] CRITICAL SECURITY VIOLATION: AWS credentials found in ${ENV_FILE}!" >&2
  echo "    The production backend MUST use the attached EC2 IAM instance profile." >&2
  exit 1
fi

# Validate environment variables without leaking secrets to stdout
# shellcheck source=/dev/null
set +e
source "$ENV_FILE"
set -e

if [ -z "${JWT_SECRET:-}" ]; then
  echo "[-] ERROR: JWT_SECRET is empty in ${ENV_FILE}. Service cannot start." >&2
  exit 1
fi

if [ "${#JWT_SECRET}" -lt 16 ]; then
  echo "[-] ERROR: JWT_SECRET must be at least 16 characters (found ${#JWT_SECRET})." >&2
  exit 1
fi

if [ "${AWS_REGION:-}" != "$TARGET_REGION" ]; then
  echo "[-] ERROR: AWS_REGION in ${ENV_FILE} is '${AWS_REGION:-}', expected '${TARGET_REGION}'." >&2
  exit 1
fi

if [ "${DYNAMODB_TABLE:-}" != "Onusly" ]; then
  echo "[-] ERROR: DYNAMODB_TABLE in ${ENV_FILE} is '${DYNAMODB_TABLE:-}', expected 'Onusly'." >&2
  exit 1
fi
echo "[+] Environment configuration verified (JWT_SECRET length >= 16, Region: ${TARGET_REGION}, Table: Onusly)."

# ------------------------------------------------------------------------------
# 8. Strict Production TLS Certificate & Key Validation
# ------------------------------------------------------------------------------
echo ">>> Validating TLS origin certificates for Cloudflare Full (strict) mode..."
mkdir -p "$SSL_DIR"
chmod 755 "$SSL_DIR"

CERT_VALID=false

if [ -f "$SSL_CERT_PATH" ] && [ -f "$SSL_KEY_PATH" ]; then
  # 8.1 Check private key file permissions (Must be 0600 or 0400, owned by root)
  KEY_PERMS="$(stat -c "%a" "$SSL_KEY_PATH" 2>/dev/null || stat -f "%Lp" "$SSL_KEY_PATH" 2>/dev/null)"
  if [ "$KEY_PERMS" != "600" ] && [ "$KEY_PERMS" != "400" ]; then
    echo "[!] Fixing permissions on ${SSL_KEY_PATH} (was ${KEY_PERMS}, setting to 0600)..."
    chmod 600 "$SSL_KEY_PATH"
  fi
  chmod 644 "$SSL_CERT_PATH"
  chown root:root "$SSL_KEY_PATH" 2>/dev/null || true
  chown root:root "$SSL_CERT_PATH" 2>/dev/null || true

  # 8.2 Verify certificate has not expired
  if openssl x509 -checkend 86400 -noout -in "$SSL_CERT_PATH" >/dev/null 2>&1; then
    echo "[+] Certificate is currently valid (not expired, >= 24h validity remaining)."
  else
    echo "[-] ERROR: Certificate at ${SSL_CERT_PATH} is expired or expires within 24 hours!" >&2
    openssl x509 -in "$SSL_CERT_PATH" -noout -dates || true
    if [ "$DEV_MODE" = false ]; then
      exit 1
    fi
  fi

  # 8.3 Verify certificate covers api.onusly.com
  CERT_TEXT="$(openssl x509 -in "$SSL_CERT_PATH" -noout -text 2>/dev/null)"
  if echo "$CERT_TEXT" | grep -E -q "api\.onusly\.com|\*\.onusly\.com"; then
    echo "[+] Certificate domain coverage verified: matches api.onusly.com or *.onusly.com."
  else
    echo "[-] ERROR: Certificate does NOT cover 'api.onusly.com'!" >&2
    openssl x509 -in "$SSL_CERT_PATH" -noout -subject || true
    if [ "$DEV_MODE" = false ]; then
      exit 1
    fi
  fi

  # 8.4 Algorithm-independent public key extraction and DER comparison (supports RSA and ECC)
  TMP_VAL_DIR="$(mktemp -d /tmp/onusly_cert_val.XXXXXX)"
  chmod 700 "$TMP_VAL_DIR"

  cleanup_cert_val_tmp() {
    rm -rf "$TMP_VAL_DIR"
  }
  trap cleanup_cert_val_tmp EXIT INT TERM

  CERT_PUB_DER="${TMP_VAL_DIR}/cert_pub.der"
  KEY_PUB_DER="${TMP_VAL_DIR}/key_pub.der"

  # Extract public key from certificate in canonical DER format
  if ! openssl x509 -in "$SSL_CERT_PATH" -noout -pubkey 2>/dev/null | \
       openssl pkey -pubin -outform DER -out "$CERT_PUB_DER" 2>/dev/null; then
    echo "[-] ERROR: Failed to extract public key from certificate (${SSL_CERT_PATH})!" >&2
    cleanup_cert_val_tmp
    trap - EXIT INT TERM
    if [ "$DEV_MODE" = false ]; then
      exit 1
    fi
  elif ! openssl pkey -in "$SSL_KEY_PATH" -pubout -outform DER -out "$KEY_PUB_DER" 2>/dev/null; then
    echo "[-] ERROR: Failed to extract public key from private key (${SSL_KEY_PATH})!" >&2
    cleanup_cert_val_tmp
    trap - EXIT INT TERM
    if [ "$DEV_MODE" = false ]; then
      exit 1
    fi
  elif cmp -s "$CERT_PUB_DER" "$KEY_PUB_DER"; then
    KEY_TYPE="$(openssl pkey -in "$SSL_KEY_PATH" -text -noout 2>/dev/null | head -n 1 | awk '{print $1}')"
    echo "[+] Cryptographic verification: private key matches certificate (${KEY_TYPE:-Public-Key} verified via DER comparison)."
    CERT_VALID=true
    cleanup_cert_val_tmp
    trap - EXIT INT TERM
  else
    echo "[-] ERROR: Cryptographic mismatch: private key (${SSL_KEY_PATH}) does NOT match certificate (${SSL_CERT_PATH})!" >&2
    cleanup_cert_val_tmp
    trap - EXIT INT TERM
    if [ "$DEV_MODE" = false ]; then
      exit 1
    fi
  fi
fi

if [ "$CERT_VALID" = false ]; then
  if [ "$DEV_MODE" = true ]; then
    echo "[!] DEVELOPMENT MODE: Generating bootstrap self-signed certificate for non-production testing..."
    openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
      -keyout "$SSL_KEY_PATH" \
      -out "$SSL_CERT_PATH" \
      -subj "/C=US/ST=State/L=City/O=ONUSLY-Dev/OU=Development/CN=api.onusly.com" \
      -addext "subjectAltName=DNS:api.onusly.com,DNS:onusly.com,DNS:www.onusly.com,DNS:onusly.app" >/dev/null 2>&1
    chmod 600 "$SSL_KEY_PATH"
    chmod 644 "$SSL_CERT_PATH"
    echo "[+] Development self-signed certificate generated."
  else
    echo "[-] CRITICAL DEPLOYMENT FAILURE: Valid production origin certificate required!" >&2
    echo "    Cloudflare Full (strict) requires an Origin CA certificate or publicly trusted certificate." >&2
    echo "    Actions required:" >&2
    echo "      1. In Cloudflare Dashboard -> SSL/TLS -> Origin Server -> Create Certificate" >&2
    echo "      2. Save the certificate to: ${SSL_CERT_PATH}" >&2
    echo "      3. Save the private key to: ${SSL_KEY_PATH} (chmod 600)" >&2
    echo "      4. Re-run this deployment script." >&2
    exit 1
  fi
fi

# ------------------------------------------------------------------------------
# 9. Deterministic Git Revision Validation
# ------------------------------------------------------------------------------
echo ">>> Checking git repository state at ${APP_DIR}..."
cd "$APP_DIR"

if [ -n "$TARGET_COMMIT" ]; then
  echo "[+] Enforcing target deployment commit: ${TARGET_COMMIT}"
  if ! git cat-file -e "${TARGET_COMMIT}^{commit}" 2>/dev/null; then
    echo "[+] Fetching remote commits from origin..."
    git fetch origin
  fi
  if ! git cat-file -e "${TARGET_COMMIT}^{commit}" 2>/dev/null; then
    echo "[-] ERROR: Commit ${TARGET_COMMIT} does not exist in repository!" >&2
    exit 1
  fi
  CURRENT_HEAD="$(git rev-parse HEAD)"
  if [ "$CURRENT_HEAD" != "$TARGET_COMMIT" ]; then
    echo "[+] Checking out target commit ${TARGET_COMMIT}..."
    git checkout "$TARGET_COMMIT"
  fi
fi

DEPLOYED_SHA="$(git rev-parse HEAD)"
echo "[+] Active deployment commit: ${DEPLOYED_SHA}"

# ------------------------------------------------------------------------------
# 10. Compile Backend Binary with Atomic Swap & Backup
# ------------------------------------------------------------------------------
echo ">>> Compiling Go server binary..."
if [ -f "${APP_DIR}/server" ]; then
  cp -p "${APP_DIR}/server" "${APP_DIR}/server.bak.${TIMESTAMP}"
  echo "[+] Backed up current binary to server.bak.${TIMESTAMP}."
fi

if go build -o "${APP_DIR}/server.new" ./cmd/server; then
  mv -f "${APP_DIR}/server.new" "${APP_DIR}/server"
  chmod 755 "${APP_DIR}/server"
  chown ec2-user:ec2-user "${APP_DIR}/server" 2>/dev/null || true
  echo "[+] Compilation successful: binary installed at ${APP_DIR}/server."
else
  echo "[-] ERROR: Go compilation failed! Preserving existing binary." >&2
  rm -f "${APP_DIR}/server.new"
  exit 1
fi

# ------------------------------------------------------------------------------
# 11. Install & Validate Nginx Configuration with Automatic Rollback
# ------------------------------------------------------------------------------
echo ">>> Installing and validating Nginx reverse proxy configuration..."
mkdir -p /etc/nginx/conf.d
mkdir -p /var/www/certbot
mkdir -p /var/www/onusly/frontend/dist

if [ -f "$NGINX_CONF_DEST" ]; then
  cp -p "$NGINX_CONF_DEST" "${NGINX_CONF_DEST}.bak.${TIMESTAMP}"
fi

cp -f "${APP_DIR}/deploy/nginx/onusly.conf" "$NGINX_CONF_DEST"

if nginx -t; then
  echo "[+] Nginx configuration syntax validated successfully."
  systemctl enable nginx >/dev/null 2>&1 || true
  systemctl restart nginx
  echo "[+] Nginx service restarted."
else
  echo "[-] ERROR: Nginx configuration test failed! Rolling back to previous configuration..." >&2
  if [ -f "${NGINX_CONF_DEST}.bak.${TIMESTAMP}" ]; then
    cp -f "${NGINX_CONF_DEST}.bak.${TIMESTAMP}" "$NGINX_CONF_DEST"
    if nginx -t >/dev/null 2>&1; then
      if systemctl restart nginx; then
        echo "[+] Nginx configuration successfully rolled back to previous version."
      else
        echo "[-] CRITICAL: Nginx restart failed during rollback!" >&2
      fi
    else
      echo "[-] CRITICAL: Rolled-back Nginx configuration also failed nginx -t!" >&2
    fi
  else
    echo "[-] No previous Nginx backup configuration found; removing invalid configuration file..." >&2
    rm -f "$NGINX_CONF_DEST"
    systemctl restart nginx || echo "[-] CRITICAL: Failed to restart Nginx after removing invalid config!" >&2
  fi
  exit 1
fi

# ------------------------------------------------------------------------------
# 12. Install & Reload Systemd Service with Automatic Rollback
# ------------------------------------------------------------------------------
echo ">>> Installing and reloading onusly.service..."
if [ -f "$SYSTEMD_SERVICE_DEST" ]; then
  cp -p "$SYSTEMD_SERVICE_DEST" "${SYSTEMD_SERVICE_DEST}.bak.${TIMESTAMP}"
fi

cp -f "${APP_DIR}/deploy/systemd/onusly.service" "$SYSTEMD_SERVICE_DEST"
systemctl daemon-reload
systemctl enable onusly >/dev/null 2>&1 || true
systemctl restart onusly

echo "[+] Waiting for ONUSLY backend service startup..."
sleep 2

if systemctl is-active --quiet onusly; then
  echo "[+] Service 'onusly' is active and running."
else
  echo "[-] ERROR: Service 'onusly' failed to start! Initiating rollback..." >&2
  journalctl -u onusly -n 40 --no-pager
  if [ -f "${APP_DIR}/server.bak.${TIMESTAMP}" ]; then
    echo "[+] Restoring previous binary from server.bak.${TIMESTAMP}..."
    cp -f "${APP_DIR}/server.bak.${TIMESTAMP}" "${APP_DIR}/server"
    if [ -f "${SYSTEMD_SERVICE_DEST}.bak.${TIMESTAMP}" ]; then
      echo "[+] Restoring previous systemd service configuration..."
      cp -f "${SYSTEMD_SERVICE_DEST}.bak.${TIMESTAMP}" "$SYSTEMD_SERVICE_DEST"
      systemctl daemon-reload
    fi
    if systemctl restart onusly && systemctl is-active --quiet onusly; then
      echo "[+] Successfully rolled back to previous working binary and service."
    else
      echo "[-] CRITICAL: Rollback failed! Previous binary also failed to start." >&2
      journalctl -u onusly -n 25 --no-pager
    fi
  else
    echo "[-] CRITICAL: No previous binary backup found to restore during rollback!" >&2
  fi
  exit 1
fi

# ------------------------------------------------------------------------------
# 13. Local End-to-End Health Verification
# ------------------------------------------------------------------------------
echo ">>> Performing local health checks..."

# 13.1 Verify local Go backend listener (:8080)
HEALTH_GO="$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8080/health || echo "CONN_FAILED")"
if [ "$HEALTH_GO" = "200" ]; then
  echo "[+] Local Go Server (http://127.0.0.1:8080/health) -> HTTP 200 OK"
else
  echo "[-] ERROR: Local Go Server check failed (Status: ${HEALTH_GO})! Initiating rollback..." >&2
  journalctl -u onusly -n 25 --no-pager
  if [ -f "${APP_DIR}/server.bak.${TIMESTAMP}" ]; then
    echo "[+] Restoring previous binary from server.bak.${TIMESTAMP}..."
    cp -f "${APP_DIR}/server.bak.${TIMESTAMP}" "${APP_DIR}/server"
    if [ -f "${SYSTEMD_SERVICE_DEST}.bak.${TIMESTAMP}" ]; then
      cp -f "${SYSTEMD_SERVICE_DEST}.bak.${TIMESTAMP}" "$SYSTEMD_SERVICE_DEST"
      systemctl daemon-reload
    fi
    if systemctl restart onusly && systemctl is-active --quiet onusly; then
      echo "[+] Successfully rolled back to previous working binary and service."
    else
      echo "[-] CRITICAL: Rollback failed! Restored binary failed to start." >&2
    fi
  else
    echo "[-] CRITICAL: No previous binary backup found to restore during rollback!" >&2
  fi
  exit 1
fi

# 13.2 Verify local Nginx HTTPS reverse proxy (:443 with Host: api.onusly.com)
HEALTH_HTTPS="$(curl -k -s -o /dev/null -w "%{http_code}" -H "Host: api.onusly.com" https://127.0.0.1/health || echo "CONN_FAILED")"
if [ "$HEALTH_HTTPS" = "200" ]; then
  echo "[+] Local Nginx HTTPS Proxy (https://127.0.0.1/health) -> HTTP 200 OK"
else
  echo "[-] ERROR: Local Nginx HTTPS Proxy check failed (Status: ${HEALTH_HTTPS})! Initiating rollback..." >&2
  if [ -f "${NGINX_CONF_DEST}.bak.${TIMESTAMP}" ]; then
    echo "[+] Restoring previous Nginx configuration..."
    cp -f "${NGINX_CONF_DEST}.bak.${TIMESTAMP}" "$NGINX_CONF_DEST"
    systemctl restart nginx || echo "[-] CRITICAL: Nginx restart failed during rollback!" >&2
  fi
  exit 1
fi

# 13.3 Verify HTTP-to-HTTPS redirect (:80)
REDIRECT_CODE="$(curl -s -o /dev/null -w "%{http_code}" -H "Host: api.onusly.com" http://127.0.0.1/health || echo "CONN_FAILED")"
if [ "$REDIRECT_CODE" = "301" ]; then
  echo "[+] Local Nginx Port 80 Redirect -> HTTP 301 Moved Permanently (Expected)"
else
  echo "[!] Port 80 Redirect check returned status: ${REDIRECT_CODE}"
fi

echo "=============================================================================="
echo ">>> ONUSLY Production Deployment Successful! Deployed commit: ${DEPLOYED_SHA}"
echo "=============================================================================="
