#!/usr/bin/env bash
set -euo pipefail

echo "==================================================================="
echo "   ONUSLY DESKTOP — SYSTEM-MODE INSTALLER & UNINSTALLER AUDIT      "
echo "==================================================================="

# Rebuild binaries first
go build -o desktop/onusly-agent ./desktop/agent/cmd
go build -o desktop/onusly-native-host ./desktop/native-host
go build -o desktop/onusly-installer ./desktop/installer/linux

unshare -rm bash << 'ROOT_EOF'
set -euo pipefail
export PATH="/usr/local/bin:/usr/bin:/bin:$PATH"

# Setup writable isolated system tree
mount -t tmpfs tmpfs /etc
mount -t tmpfs tmpfs /usr/local/bin
mount -t tmpfs tmpfs /run
mount -t tmpfs tmpfs /var/lib
mkdir -p /run/onusly /var/lib/onusly

# Provide writable mount for system native messaging hosts
mkdir -p /tmp/moz_native
mkdir -p /usr/lib64/mozilla/native-messaging-hosts || true
mount --bind /tmp/moz_native /usr/lib64/mozilla/native-messaging-hosts || mount -t tmpfs tmpfs /usr/lib64/mozilla/native-messaging-hosts

mkdir -p /etc/systemd/system
mkdir -p /etc/brave/policies/managed
mkdir -p /etc/brave/native-messaging-hosts
mkdir -p /etc/firefox/policies
mkdir -p /usr/lib64/mozilla/native-messaging-hosts
mkdir -p /etc/chromium/policies/managed
mkdir -p /etc/opt/chrome/policies/managed
mkdir -p /etc/opt/edge/policies/managed

# 1. Pre-seed pre-existing corporate policies
echo '{"CorporateProxy": "proxy.corp.internal:8080"}' > /etc/brave/policies/managed/onusly.json
echo '{"policies": {"CorpCertAllowed": true}}' > /etc/firefox/policies/policies.json

echo "[Step 1] Pre-existing corporate policies staged:"
cat /etc/brave/policies/managed/onusly.json
cat /etc/firefox/policies/policies.json
echo

# 2. Run system-mode installation
echo "[Step 2] Executing system-mode installer (as root UID $(id -u))..."
./desktop/onusly-installer --yes

# 3. Verify installed artifacts
echo -e "\n[Step 3] Verifying installed artifacts and policy configurations..."
test -f /usr/local/bin/onusly-agent || { echo "FAIL: agent missing"; exit 1; }
test -f /usr/local/bin/onusly-native-host || { echo "FAIL: native-host missing"; exit 1; }
test -f /etc/systemd/system/onusly-desktop.service || { echo "FAIL: systemd service missing"; exit 1; }

echo "  • Systemd service exists:"
head -n 6 /etc/systemd/system/onusly-desktop.service

echo "  • Brave managed policy merged:"
cat /etc/brave/policies/managed/onusly.json
python3 -c '
import json
with open("/etc/brave/policies/managed/onusly.json") as f:
    d = json.load(f)
assert d.get("CorporateProxy") == "proxy.corp.internal:8080", "Corporate policy lost!"
assert d.get("IncognitoModeAvailability") == 1, "Incognito not disabled!"
assert any("gkichkblphmdbojdjnonljlegnmjcnhg" in x for x in d.get("ExtensionInstallForcelist", [])), "Extension not force installed!"
print("    -> Brave policy assertions PASSED (Corporate policy preserved + Extension force installed + Incognito disabled)")
'

echo "  • Firefox enterprise policy merged:"
cat /etc/firefox/policies/policies.json
python3 -c '
import json
with open("/etc/firefox/policies/policies.json") as f:
    d = json.load(f)["policies"]
assert d.get("CorpCertAllowed") == True, "Corp cert policy lost!"
assert d.get("DisablePrivateBrowsing") == True, "Private browsing not disabled in Firefox!"
ext = d["ExtensionSettings"]["extension@onusly.com"]
assert ext.get("installation_mode") == "force_installed", "Firefox extension not force installed!"
print("    -> Firefox policy assertions PASSED (Corporate setting preserved + Extension force installed + Private browsing disabled)")
'

# 4. Run uninstaller
echo -e "\n[Step 4] Executing system-mode uninstaller..."
./desktop/onusly-installer --uninstall

# 5. Verify complete rollback & restoration
echo -e "\n[Step 5] Verifying restoration of pre-existing state..."
if [ -f /etc/systemd/system/onusly-desktop.service ]; then
    echo "FAIL: systemd service was not removed!"
    exit 1
fi
echo "  • Systemd service cleanly removed."

python3 -c '
import json
with open("/etc/brave/policies/managed/onusly.json") as f:
    d = json.load(f)
assert d.get("CorporateProxy") == "proxy.corp.internal:8080", "Corporate policy not restored!"
assert "ExtensionInstallForcelist" not in d, "Extension force install not cleaned up!"
print("  • Brave policy cleanly restored to pre-install corporate state.")
'

python3 -c '
import json
with open("/etc/firefox/policies/policies.json") as f:
    d = json.load(f)["policies"]
assert d.get("CorpCertAllowed") == True, "Corp cert policy not restored!"
assert "ExtensionSettings" not in d, "Firefox ExtensionSettings not cleaned up!"
print("  • Firefox policy cleanly restored to pre-install corporate state.")
'

echo -e "\n🎉 SYSTEM MODE INSTALLER & UNINSTALLER FULLY VERIFIED."
ROOT_EOF
