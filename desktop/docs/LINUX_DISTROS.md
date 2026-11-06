# ONUSLY Desktop — Linux Distribution Adapters & Installation Guide

**Target Architecture:** One Installation → One Account → One Agent → All Supported Browsers  
**Supported Distributions:** Fedora (Primary), Debian, Ubuntu, and generic systemd Linux.

---

## 1. Browser Directory Layouts

### Chromium-Family Browsers

| Browser | Executable Candidates | System Managed Policies | System Native Hosts | User Native Hosts (`~/.config/...`) |
|---|---|---|---|---|
| **Google Chrome** | `google-chrome`, `chrome` | `/etc/opt/chrome/policies/managed` | `/etc/opt/chrome/native-messaging-hosts` | `~/.config/google-chrome/NativeMessagingHosts` |
| **Chromium** | `chromium-browser`, `chromium` | `/etc/chromium/policies/managed` | `/etc/chromium/native-messaging-hosts` | `~/.config/chromium/NativeMessagingHosts` |
| **Brave** | `brave-browser`, `brave` | `/etc/brave/policies/managed` | `/etc/brave/native-messaging-hosts` | `~/.config/BraveSoftware/Brave-Browser/NativeMessagingHosts` |
| **Microsoft Edge** | `microsoft-edge`, `microsoft-edge-stable` | `/etc/opt/edge/policies/managed` | `/etc/opt/edge/native-messaging-hosts` | `~/.config/microsoft-edge/NativeMessagingHosts` |

### Mozilla Firefox

| Distribution / Packaging | Executable Candidates | Managed Policies Directory | Native Messaging Manifests |
|---|---|---|---|
| **Fedora / RHEL (RPM)** | `firefox` | `/etc/firefox/policies/policies.json` | `/usr/lib64/mozilla/native-messaging-hosts` / `~/.mozilla/native-messaging-hosts` |
| **Debian (APT)** | `firefox`, `firefox-esr` | `/etc/firefox/policies/policies.json` | `/usr/lib/mozilla/native-messaging-hosts` / `~/.mozilla/native-messaging-hosts` |
| **Ubuntu (Snap)** | `firefox` | `/etc/firefox/policies/policies.json` (host policy is inherited by modern Snap Firefox) | `~/.mozilla/native-messaging-hosts` |

---

## 2. Installation Modes

### 2.1 System Mode (Root / Administrator)
Installs systemd system service and enterprise-level tamper-proof browser policies across all users.

```bash
sudo ./desktop/onusly-installer --yes
```

**Actions:**
1. Copies binaries to `/usr/local/bin/onusly-agent` and `/usr/local/bin/onusly-native-host`.
2. Creates `/etc/onusly` configuration and `/var/lib/onusly` data directories with restricted permissions (`0700`).
3. Writes enterprise policy files (`onusly.json` and `policies.json`) in `/etc/<browser>/policies/managed` to force-install the ONUSLY extension and disable incognito bypass.
4. Writes native messaging host manifests in `/etc/<browser>/native-messaging-hosts/com.onusly.desktop_agent.json`.
5. Creates `/etc/systemd/system/onusly-desktop.service` and activates via `systemctl enable --now onusly-desktop`.

### 2.2 User Mode (Non-Root / Development)
Installs for the current user without requiring sudo or modifying `/etc`.

```bash
./desktop/onusly-installer --yes --user
```

**Actions:**
1. Copies binaries to `~/.local/bin/onusly-agent` and `~/.local/bin/onusly-native-host`.
2. Creates `~/.config/onusly` configuration and `~/.local/share/onusly` data directories (`0700`).
3. Registers native messaging host manifests in user directories:
   - `~/.config/BraveSoftware/Brave-Browser/NativeMessagingHosts/`
   - `~/.config/google-chrome/NativeMessagingHosts/`
   - `~/.config/chromium/NativeMessagingHosts/`
   - `~/.config/microsoft-edge/NativeMessagingHosts/`
   - `~/.mozilla/native-messaging-hosts/`
4. Browser extension connects to `~/.config/onusly/agent.sock` via the registered native host.
5. Notes `policy_missing` telemetry issue as expected when running without root privileges.

---

## 3. Account Pairing & Daemon Operation

### Step 1: Request Pairing
```bash
~/.local/bin/onusly-agent pair --api-url=http://localhost:8080
```
Outputs a 6-character code (e.g. `ABC123`).

### Step 2: Confirm in Dashboard
Open the ONUSLY Dashboard (`http://localhost:5173`) and enter the 6-character code under **Active Device Coverage** -> **Pair Desktop Agent**.

### Step 3: Start Agent Daemon
```bash
~/.local/bin/onusly-agent run
```

---

## 4. Uninstallation & Rollback

To cleanly restore original browser configurations, remove native manifests, and stop services:

```bash
# User mode uninstallation
./desktop/onusly-installer --uninstall --user

# System mode uninstallation
sudo ./desktop/onusly-installer --uninstall
```
