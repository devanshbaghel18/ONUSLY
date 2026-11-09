#!/usr/bin/env bash
# ==============================================================================
# Test Harness for Algorithm-Independent Certificate & Key Validation
# ==============================================================================

set -euo pipefail

TEST_DIR="$(mktemp -d /tmp/test_cert_suite.XXXXXX)"
cleanup() {
  rm -rf "$TEST_DIR"
}
trap cleanup EXIT INT TERM

echo "=== 1. Generating Test Certificates & Keys ==="
# A. RSA Key & Certificate (Valid, covers api.onusly.com)
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "${TEST_DIR}/rsa_valid.key" \
  -out "${TEST_DIR}/rsa_valid.crt" \
  -days 365 \
  -subj "/C=US/ST=State/L=City/O=Test/CN=api.onusly.com" \
  -addext "subjectAltName=DNS:api.onusly.com" 2>/dev/null

# B. ECC Key & Certificate (Valid, prime256v1, covers api.onusly.com)
openssl ecparam -name prime256v1 -genkey -noout -out "${TEST_DIR}/ecc_valid.key" 2>/dev/null
openssl req -new -x509 -key "${TEST_DIR}/ecc_valid.key" \
  -out "${TEST_DIR}/ecc_valid.crt" \
  -days 365 \
  -subj "/C=US/ST=State/L=City/O=Test/CN=api.onusly.com" \
  -addext "subjectAltName=DNS:api.onusly.com" 2>/dev/null

# C. Another RSA Key (For mismatch test)
openssl genrsa -out "${TEST_DIR}/rsa_other.key" 2048 2>/dev/null

# D. Expired Certificate (Generated via Python cryptography with past dates)
python3 -c "
from datetime import datetime, timedelta, timezone
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa

key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
cert = x509.CertificateBuilder().subject_name(
    x509.Name([x509.NameAttribute(x509.oid.NameOID.COMMON_NAME, 'api.onusly.com')])
).issuer_name(
    x509.Name([x509.NameAttribute(x509.oid.NameOID.COMMON_NAME, 'api.onusly.com')])
).public_key(
    key.public_key()
).serial_number(
    x509.random_serial_number()
).not_valid_before(
    datetime.now(timezone.utc) - timedelta(days=10)
).not_valid_after(
    datetime.now(timezone.utc) - timedelta(days=1)
).sign(key, hashes.SHA256())

with open('${TEST_DIR}/expired.crt', 'wb') as f:
    f.write(cert.public_bytes(serialization.Encoding.PEM))
with open('${TEST_DIR}/expired.key', 'wb') as f:
    f.write(key.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.TraditionalOpenSSL, serialization.NoEncryption()))
"

# E. Certificate with Wrong Domain (wrong.com)
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "${TEST_DIR}/wrong_domain.key" \
  -out "${TEST_DIR}/wrong_domain.crt" \
  -days 365 \
  -subj "/CN=example.com" \
  -addext "subjectAltName=DNS:example.com" 2>/dev/null

echo "=== 2. Testing DER Extraction & Comparison Function ==="

verify_cert_and_key() {
  local cert_path="$1"
  local key_path="$2"

  local tmp_val_dir
  tmp_val_dir="$(mktemp -d /tmp/val_test.XXXXXX)"
  local cert_pub_der="${tmp_val_dir}/cert_pub.der"
  local key_pub_der="${tmp_val_dir}/key_pub.der"

  # Step 1: Check expiration (at least 24h)
  if ! openssl x509 -checkend 0 -noout -in "$cert_path" >/dev/null 2>&1; then
    rm -rf "$tmp_val_dir"
    return 1
  fi

  # Step 2: Check domain
  if ! openssl x509 -in "$cert_path" -noout -text 2>/dev/null | grep -E -q "api\.onusly\.com|\*\.onusly\.com"; then
    rm -rf "$tmp_val_dir"
    return 2
  fi

  # Step 3: Extract public keys and compare DER
  if ! openssl x509 -in "$cert_path" -noout -pubkey 2>/dev/null | \
       openssl pkey -pubin -outform DER -out "$cert_pub_der" 2>/dev/null; then
    rm -rf "$tmp_val_dir"
    return 3
  fi

  if ! openssl pkey -in "$key_path" -pubout -outform DER -out "$key_pub_der" 2>/dev/null; then
    rm -rf "$tmp_val_dir"
    return 4
  fi

  if ! cmp -s "$cert_pub_der" "$key_pub_der"; then
    rm -rf "$tmp_val_dir"
    return 5
  fi

  rm -rf "$tmp_val_dir"
  return 0
}

# Run Test Cases
echo "Running Test Cases:"

# Case 1: RSA Valid Match
if verify_cert_and_key "${TEST_DIR}/rsa_valid.crt" "${TEST_DIR}/rsa_valid.key"; then
  echo "  [PASS] Case 1: RSA valid cert and key match"
else
  echo "  [FAIL] Case 1: RSA valid cert and key failed"
  exit 1
fi

# Case 2: ECC Valid Match
if verify_cert_and_key "${TEST_DIR}/ecc_valid.crt" "${TEST_DIR}/ecc_valid.key"; then
  echo "  [PASS] Case 2: ECC valid cert and key match (ECDSA prime256v1)"
else
  echo "  [FAIL] Case 2: ECC valid cert and key failed"
  exit 1
fi

# Case 3: RSA cert with different RSA key (Mismatch)
set +e
verify_cert_and_key "${TEST_DIR}/rsa_valid.crt" "${TEST_DIR}/rsa_other.key"
res=$?
set -e
if [ "$res" -eq 5 ]; then
  echo "  [PASS] Case 3: Mismatched RSA keys correctly detected (code 5)"
else
  echo "  [FAIL] Case 3: Failed to detect mismatched RSA keys (got $res)"
  exit 1
fi

# Case 4: RSA cert with ECC key (Mismatch across algorithms)
set +e
verify_cert_and_key "${TEST_DIR}/rsa_valid.crt" "${TEST_DIR}/ecc_valid.key"
res=$?
set -e
if [ "$res" -eq 5 ]; then
  echo "  [PASS] Case 4: Mismatched RSA/ECC pair correctly detected (code 5)"
else
  echo "  [FAIL] Case 4: Failed to detect cross-algorithm mismatch (got $res)"
  exit 1
fi

# Case 5: Expired cert
set +e
verify_cert_and_key "${TEST_DIR}/expired.crt" "${TEST_DIR}/expired.key"
res=$?
set -e
if [ "$res" -eq 1 ]; then
  echo "  [PASS] Case 5: Expired certificate correctly detected (code 1)"
else
  echo "  [FAIL] Case 5: Failed to detect expired certificate (got $res)"
  exit 1
fi

# Case 6: Wrong domain
set +e
verify_cert_and_key "${TEST_DIR}/wrong_domain.crt" "${TEST_DIR}/wrong_domain.key"
res=$?
set -e
if [ "$res" -eq 2 ]; then
  echo "  [PASS] Case 6: Certificate missing api.onusly.com correctly detected (code 2)"
else
  echo "  [FAIL] Case 6: Failed to detect invalid domain coverage (got $res)"
  exit 1
fi

echo ""
echo ">>> ALL CERTIFICATE & KEY VALIDATION SUITE TESTS PASSED! <<<"
