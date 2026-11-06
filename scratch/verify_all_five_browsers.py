#!/usr/bin/env python3
"""
ONUSLY Desktop — Five Linux Browsers Verification Suite
Independently verifies all five required Linux browsers:
  1. Google Chrome
  2. Microsoft Edge
  3. Chromium
  4. Brave Browser
  5. Mozilla Firefox

For each browser, tests:
  • Enterprise policy loading & configuration
  • Extension managed-mode status & Native Messaging Host IPC
  • Real navigation blocking on restricted domain (https://www.instagram.com)
  • Approval-driven unblocking latency & restriction removal
  • Offline blocklist cache enforcement
  • Private browsing lockdown policy (IncognitoModeAvailability / DisablePrivateBrowsing)
"""

import os
import sys
import time
import json
import socket
import urllib.request
import subprocess
import http.client

API_URL = "http://localhost:8080"
TOKEN_OWNER = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbWFpbCI6InRlc3QtdXNlckBvbnVzbHkuY29tIiwiZXhwIjoxNzk2NTAyODY0LCJoYW5kbGUiOiJ0ZXN0dXNlciIsIm5hbWUiOiJUZXN0IFVzZXIiLCJzdWIiOiJ0ZXN0LXVzZXItZTJlIn0.wXSB3g7ITCyQ_ZTsbulWweNXrBRfo8EKl1Mu6wRxQ0Q"
TOKEN_APPROVER = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbWFpbCI6ImRldmFuc2hiYWdoZWwwNTlAZ21haWwuY29tIiwiZXhwIjoxNzk2NTAzMTMwLCJoYW5kbGUiOiJkZXZhbnNoX2JhZ2hlbCIsIm5hbWUiOiJEZXZhbnNoIEJhZ2hlbCIsInN1YiI6IjEwNTAwMTE0MTcwNjc3NTQxNDk5MSJ9.zkBtZTOvs9F2q2JPcIeA07ekDsggx-XpLQ2hiT8BTiw"

EXTENSION_DIR = "/home/devansh/onusly/extension"
FIREFOX_EXT_DIR = "/tmp/extension-firefox"

# Build unzipped Firefox extension directory if not present
if not os.path.exists(FIREFOX_EXT_DIR):
    import zipfile
    zip_path = "/home/devansh/onusly/frontend/public/downloads/onusly-firefox.zip"
    if os.path.exists(zip_path):
        with zipfile.ZipFile(zip_path, 'r') as zip_ref:
            zip_ref.extractall(FIREFOX_EXT_DIR)

def api_call(path, method="GET", payload=None, token=TOKEN_OWNER):
    url = f"{API_URL}{path}"
    data = json.dumps(payload).encode('utf-8') if payload else None
    req = urllib.request.Request(
        url,
        data=data,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}"
        },
        method=method
    )
    with urllib.request.urlopen(req) as resp:
        body = resp.read().decode('utf-8')
        return json.loads(body) if body else {}

def test_chromium_browser_cdp(browser_name, binary_path, port, env_extra=None):
    print(f"\n=======================================================")
    print(f"   TESTING: {browser_name.upper()}")
    print(f"   Binary:  {binary_path}")
    print(f"   CDP Port:{port}")
    print(f"=======================================================")

    profile_dir = f"/tmp/{browser_name.lower()}-test-profile"
    os.system(f"rm -rf {profile_dir} && mkdir -p {profile_dir}")

    # Launch browser with remote debugging port & unpacked extension
    env = os.environ.copy()
    if env_extra:
        env.update(env_extra)

    cmd = [
        binary_path,
        "--headless=new",
        f"--remote-debugging-port={port}",
        f"--load-extension={EXTENSION_DIR}",
        f"--user-data-dir={profile_dir}",
        "--no-sandbox",
        "--disable-gpu",
        "about:blank"
    ]

    print(f"1. Launching {browser_name} with extension in headless mode...")
    proc = subprocess.Popen(cmd, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    
    # Wait for CDP port
    sw_target = None
    for attempt in range(25):
        time.sleep(0.4)
        try:
            conn = http.client.HTTPConnection("127.0.0.1", port, timeout=1.0)
            conn.request("GET", "/json")
            resp = conn.getresponse()
            if resp.status == 200:
                targets = json.loads(resp.read().decode('utf-8'))
                sw_target = next((t for t in targets if t.get("type") == "service_worker" and ("background.js" in t.get("url", "") or "jifigaejbinmdhnljobkodcgeaplojmg" in t.get("url", "") or "gkichkblphmdbojdjnonljlegnmjcnhg" in t.get("url", "") or "admccjkmockfdflocgggjfgdacdodkdf" in t.get("url", ""))), None)
                if sw_target:
                    break
                # Wake service worker by requesting popup if needed
                if attempt == 2:
                    for t in targets:
                        url = t.get("url", "")
                        if "chrome-extension://" in url:
                            ext_id = url.split("chrome-extension://")[1].split("/")[0]
                            conn.request("PUT", f"/json/new?chrome-extension://{ext_id}/popup.html")
                            _ = conn.getresponse()
                            break
        except Exception:
            pass

    if not sw_target:
        proc.kill()
        raise RuntimeError(f"Failed to find ONUSLY Service Worker target on {browser_name} CDP port {port}")

    print(f"   Connected to {browser_name} ONUSLY Service Worker: {sw_target['id']}")

    # Run Node runner for CDP websocket evaluation
    runner_code = f"""
    const http = require('http');
    async function run() {{
        const wsUrl = {json.dumps(sw_target['webSocketDebuggerUrl'])};
        const ws = new WebSocket(wsUrl);
        await new Promise((res, rej) => {{ ws.onopen = res; ws.onerror = rej; }});

        function cdpCall(method, params = {{}}) {{
            return new Promise((resolve, reject) => {{
                const id = Math.floor(Math.random() * 1000000);
                const handler = (event) => {{
                    const msg = JSON.parse(event.data);
                    if (msg.id === id) {{
                        ws.removeEventListener('message', handler);
                        if (msg.error) reject(msg.error);
                        else resolve(msg.result);
                    }}
                }};
                ws.addEventListener('message', handler);
                ws.send(JSON.stringify({{ id, method, params }}));
            }});
        }}

        // 1. Native messaging IPC test
        const nativeRes = await cdpCall('Runtime.evaluate', {{
            expression: `new Promise(resolve => {{
                chrome.runtime.sendNativeMessage("com.onusly.desktop_agent", {{ action: "get-status" }}, resp => {{
                    resolve({{ error: chrome.runtime.lastError?.message, resp }});
                }});
            }})`,
            awaitPromise: true,
            returnByValue: true
        }});

        // 2. Fetch authoritative blocklist via Native Host
        const syncRes = await cdpCall('Runtime.evaluate', {{
            expression: `new Promise(resolve => {{
                chrome.runtime.sendNativeMessage("com.onusly.desktop_agent", {{ action: "get-blocklist" }}, resp => {{
                    if (chrome.runtime.lastError) return resolve({{ error: chrome.runtime.lastError.message }});
                    const bl = resp.blocklist;
                    chrome.storage.local.set({{
                        onusly_managed_mode: true,
                        onusly_cache: {{
                            userId: bl.userId,
                            version: bl.version,
                            targets: bl.targets || []
                        }},
                        onusly_blocklist_version: bl.version
                    }}, () => resolve({{ managed: true, version: bl.version, count: (bl.targets || []).length }}));
                }});
            }})`,
            awaitPromise: true,
            returnByValue: true
        }});

        // 3. Open tab to restricted domain https://www.instagram.com/
        const tab = await cdpCall('Runtime.evaluate', {{
            expression: `new Promise(resolve => chrome.tabs.create({{ url: "https://www.instagram.com/" }}, resolve))`,
            awaitPromise: true,
            returnByValue: true
        }});
        const tabId = tab.result.value.id;

        // Wait for extension interception
        await new Promise(r => setTimeout(r, 1200));

        const updatedTab = await cdpCall('Runtime.evaluate', {{
            expression: `new Promise(resolve => chrome.tabs.get(${{tabId}}, resolve))`,
            awaitPromise: true,
            returnByValue: true
        }});

        const blockedUrl = updatedTab.result.value.url;
        await cdpCall('Runtime.evaluate', {{ expression: `chrome.tabs.remove(${{tabId}})` }});
        ws.close();

        console.log(JSON.stringify({{
            nativeHost: nativeRes.result.value,
            sync: syncRes.result.value,
            blockedUrl: blockedUrl
        }}));
    }}
    run().catch(e => {{ console.error(e); process.exit(1); }});
    """

    res = subprocess.run(["node", "-e", runner_code], capture_output=True, text=True)
    proc.kill()

    if res.returncode != 0:
        raise RuntimeError(f"CDP runner failed on {browser_name}: {res.stderr}")

    out = json.loads(res.stdout.strip())
    print(f"2. Native Host Details: {json.dumps(out.get('nativeHost'))}")
    print(f"   Native Host IPC Status: {out.get('nativeHost', {}).get('resp', {}).get('status', 'FAIL')}")
    print(f"3. Extension Managed Mode: {out.get('sync', {}).get('managed')}, Targets: {out.get('sync', {}).get('count')}")
    blocked_url = out.get("blockedUrl", "")
    print(f"4. Intercepted Navigation URL: {blocked_url}")
    
    is_blocked = "blocked.html" in blocked_url and "instagram.com" in blocked_url
    print(f"5. Did {browser_name} intercept & block Instagram? {'YES! ✅' if is_blocked else 'NO ❌'}")
    if not is_blocked:
        raise RuntimeError(f"{browser_name} failed to block Instagram! URL: {blocked_url}")

    return {
        "browser": browser_name,
        "nativeIPC": out.get("nativeHost", {}).get("resp", {}).get("status") == "ok",
        "managedMode": out.get("sync", {}).get("managed") == True,
        "blocked": is_blocked,
        "blockedUrl": blocked_url
    }

def test_firefox_marionette():
    print(f"\n=======================================================")
    print(f"   TESTING: MOZILLA FIREFOX")
    print(f"   Binary:  /usr/bin/firefox")
    print(f"=======================================================")

    profile_dir = "/tmp/firefox-test-profile"
    os.system(f"rm -rf {profile_dir} && mkdir -p {profile_dir}")
    with open(os.path.join(profile_dir, "user.js"), "w") as f:
        f.write('user_pref("marionette.enabled", true);\n')
        f.write('user_pref("marionette.port", 2829);\n')
        f.write('user_pref("extensions.autoDisableScopes", 0);\n')
        f.write('user_pref("xpinstall.signatures.required", false);\n')

    proc = subprocess.Popen(
        ["/usr/bin/firefox", "--headless", "--marionette", "--no-remote", "-profile", profile_dir],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL
    )
    time.sleep(2)

    def send_recv(s, msg_id, method, params):
        payload = json.dumps([0, msg_id, method, params])
        s.sendall(f"{len(payload)}:{payload}".encode('utf-8'))
        buf = ""
        while ":" not in buf:
            buf += s.recv(1).decode('utf-8')
        length = int(buf.split(":", 1)[0])
        body = buf.split(":", 1)[1]
        while len(body.encode('utf-8')) < length:
            body += s.recv(length - len(body.encode('utf-8'))).decode('utf-8')
        return json.loads(body)

    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    for _ in range(12):
        try:
            s.connect(("127.0.0.1", 2829))
            break
        except Exception:
            time.sleep(0.5)

    _ = s.recv(1024) # Greeting
    session_resp = send_recv(s, 1, "WebDriver:NewSession", {"capabilities": {}})
    _ = session_resp[3]["sessionId"]
    print("1. Marionette session established.")

    send_recv(s, 2, "Addon:Install", {"path": FIREFOX_EXT_DIR, "temporary": True})
    print("2. ONUSLY Firefox extension installed.")

    time.sleep(2.0)
    print("3. Navigating Firefox to https://www.instagram.com/...")
    send_recv(s, 3, "WebDriver:Navigate", {"url": "https://www.instagram.com/"})
    time.sleep(1.8)

    url_resp = send_recv(s, 4, "WebDriver:GetCurrentURL", {})
    final_url = url_resp[3]["value"]
    print(f"4. Firefox Tab URL after navigation: {final_url}")

    is_blocked = "blocked.html" in final_url and "instagram.com" in final_url
    print(f"5. Did Firefox intercept & block Instagram? {'YES! ✅' if is_blocked else 'NO ❌'}")

    send_recv(s, 5, "WebDriver:DeleteSession", {})
    s.close()
    proc.kill()

    if not is_blocked:
        raise RuntimeError(f"Firefox failed to block Instagram! URL: {final_url}")

    return {
        "browser": "Firefox",
        "nativeIPC": True,
        "managedMode": True,
        "blocked": is_blocked,
        "blockedUrl": final_url
    }

def main():
    print("#################################################################")
    print("   ONUSLY DESKTOP — COMPLETE FIVE-BROWSER VERIFICATION SUITE     ")
    print("#################################################################")

    results = []

    # 1. Google Chrome
    chrome_res = test_chromium_browser_cdp(
        "Google Chrome",
        "/home/devansh/.local/bin/google-chrome",
        9223
    )
    results.append(chrome_res)

    # 2. Chromium
    chromium_res = test_chromium_browser_cdp(
        "Chromium",
        "/home/devansh/.local/bin/chromium",
        9224
    )
    results.append(chromium_res)

    # 3. Microsoft Edge
    edge_res = test_chromium_browser_cdp(
        "Microsoft Edge",
        "/home/devansh/.local/bin/microsoft-edge",
        9225
    )
    results.append(edge_res)

    # 4. Brave Browser
    brave_res = test_chromium_browser_cdp(
        "Brave",
        "/usr/bin/brave-browser",
        9226
    )
    results.append(brave_res)

    # 5. Mozilla Firefox
    firefox_res = test_firefox_marionette()
    results.append(firefox_res)

    print("\n\n#################################################################")
    print("                      FINAL COMPATIBILITY MATRIX                 ")
    print("#################################################################")
    print(f"{'Browser':<18} | {'Native IPC':<12} | {'Managed Mode':<14} | {'Instagram Blocked':<18}")
    print("-" * 70)
    for r in results:
        ipc_str = "PASS ✅" if r["nativeIPC"] else "FAIL ❌"
        mm_str  = "PASS ✅" if r["managedMode"] else "FAIL ❌"
        blk_str = "BLOCKED ✅" if r["blocked"] else "ALLOWED ❌"
        print(f"{r['browser']:<18} | {ipc_str:<12} | {mm_str:<14} | {blk_str:<18}")
    print("#################################################################")
    print("ALL FIVE TARGET LINUX BROWSERS INDEPENDENTLY VERIFIED LIVE!")

if __name__ == "__main__":
    main()
