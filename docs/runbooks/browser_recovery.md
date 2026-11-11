# Runbook: Browser Integration & Native Messaging Recovery

## 1. Overview
The ONUSLY Browser Extension interacts with the ONUSLY Desktop Agent via a Native Messaging Host binary (`onusly-native-host.exe`).
- **Framing Protocol:** Chrome/Edge native messaging framing (32-bit unsigned little-endian length prefix followed by UTF-8 JSON payload).
- **Security Boundary:** The native host runs in the logged-on user session with low rights and connects to the desktop agent over `\\.\pipe\OnuslyAgentIPC`.
- **Credential Isolation:** The native host has zero access to device credentials, DPAPI master keys, or backend session tokens. It receives only sanitised policy statuses from the desktop agent.

---

## 2. Registry Registration Invariants

### 2.1 Manifest Paths
- **Chrome:** `HKLM:\SOFTWARE\Google\Chrome\NativeMessagingHosts\com.onusly.native_host`
- **Edge:** `HKLM:\SOFTWARE\Microsoft\Edge\NativeMessagingHosts\com.onusly.native_host`
- Target JSON Manifest: `C:\Program Files\Onusly\com.onusly.native_host.json`

Verify Manifest Registration:
```powershell
Get-ItemProperty "HKLM:\SOFTWARE\Google\Chrome\NativeMessagingHosts\com.onusly.native_host" "(Default)"
Get-ItemProperty "HKLM:\SOFTWARE\Microsoft\Edge\NativeMessagingHosts\com.onusly.native_host" "(Default)"
```
Expected value: `C:\Program Files\Onusly\com.onusly.native_host.json`

### 2.2 Enterprise Force-Install Policies (Optional / Managed Environments)
- **Chrome:** `HKLM:\SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist`
- **Edge:** `HKLM:\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallForcelist`

---

## 3. Diagnostics & Troubleshooting

### 3.1 Extension Cannot Connect to Host (Error: `Specified native messaging host not found`)
1. Check that `com.onusly.native_host.json` exists in `C:\Program Files\Onusly\`.
2. Inspect manifest permissions and content:
   ```json
   {
     "name": "com.onusly.native_host",
     "description": "ONUSLY Native Messaging Host",
     "path": "C:\\Program Files\\Onusly\\onusly-native-host.exe",
     "type": "stdio",
     "allowed_origins": [
       "chrome-extension://knldjmfmopnplhmkfmgbedebdepgaddn/",
       "chrome-extension://*enforcement-extension-id*/"
     ]
   }
   ```
3. Verify the executable path in the manifest is absolute and points to the actual installed binary.

### 3.2 Native Host Fails to Connect to Agent (Error: `IPC Pipe Error`)
1. Test pipe connectivity from user console:
   ```powershell
   & "C:\Program Files\Onusly\onusly-agent.exe" query status
   ```
2. If pipe access is denied:
   - Verify that the logged-on user or Authenticated Users group has read/write permissions to the named pipe DACL.
   - Verify that the desktop agent service is running as `NT AUTHORITY\LocalService`.

### 3.3 Diagnostic End-to-End Test
Send a synthetic native messaging frame directly to the host executable:
```powershell
# Run the test_pairing_acceptance.ps1 native host check
& "C:\Program Files\Onusly\onusly-native-host.exe" -test
```
Or execute the automated validation step:
```powershell
powershell -ExecutionPolicy Bypass -Command {
    $proc = Start-Process -FilePath "C:\Program Files\Onusly\onusly-native-host.exe" -PassThru -NoNewWindow -RedirectStandardInput input.dat -RedirectStandardOutput output.dat
}
```

---

## 4. Re-Registration Procedure
To reinstall or repair native messaging manifests without reinstalling the entire service:
```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
.\desktop\installer\windows\install.ps1 -Repair -Verbose
```
This repairs registry keys, recreates manifests, and revalidates file permissions.

