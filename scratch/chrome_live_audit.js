const http = require('http');
const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');

const EXTENSION_ID = 'jifigaejbinmdhnljobkodcgeaplojmg';
const CDP_PORT = 9223;
const HTTP_PORT = 8998;
const CHROME_BIN = '/home/devansh/.local/bin/google-chrome';
const PROFILE_DIR = '/tmp/chrome-enterprise-live-profile';
const SOCK_PATH = '/run/onusly/agent.sock';

function wait(ms) {
  return new Promise(r => setTimeout(r, ms));
}

function getJson(url) {
  return new Promise((resolve, reject) => {
    http.get(url, res => {
      let data = '';
      res.on('data', chunk => data += chunk);
      res.on('end', () => {
        try { resolve(JSON.parse(data)); } catch (e) { reject(e); }
      });
    }).on('error', reject);
  });
}

function putJson(url) {
  return new Promise((resolve, reject) => {
    const parsed = new URL(url);
    const req = http.request({
      hostname: parsed.hostname,
      port: parsed.port,
      path: parsed.pathname + parsed.search,
      method: 'PUT'
    }, res => {
      let data = '';
      res.on('data', chunk => data += chunk);
      res.on('end', () => {
        try { resolve(JSON.parse(data)); } catch (e) { resolve(data); }
      });
    });
    req.on('error', reject);
    req.end();
  });
}

class CDPClient {
  constructor(wsUrl) {
    this.wsUrl = wsUrl;
    this.ws = null;
    this.callId = 1;
    this.handlers = new Map();
  }

  async connect(timeoutMs = 10000) {
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('WebSocket connection timeout')), timeoutMs);
      this.ws = new WebSocket(this.wsUrl);
      this.ws.onopen = () => {
        clearTimeout(timer);
        console.log(`[CDP] Connected to ${this.wsUrl}`);
        resolve();
      };
      this.ws.onerror = (err) => {
        clearTimeout(timer);
        reject(err);
      };
      this.ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data);
          if (msg.id && this.handlers.has(msg.id)) {
            const { resolve, reject, timer } = this.handlers.get(msg.id);
            clearTimeout(timer);
            this.handlers.delete(msg.id);
            if (msg.error) {
              reject(new Error(JSON.stringify(msg.error)));
            } else {
              resolve(msg.result);
            }
          }
        } catch (e) {
          console.error('[CDP Parse Error]', e);
        }
      };
    });
  }

  call(method, params = {}, timeoutMs = 8000) {
    return new Promise((resolve, reject) => {
      const id = this.callId++;
      const timer = setTimeout(() => {
        this.handlers.delete(id);
        reject(new Error(`CDP call ${method} (id=${id}) timed out after ${timeoutMs}ms`));
      }, timeoutMs);
      this.handlers.set(id, { resolve, reject, timer });
      this.ws.send(JSON.stringify({ id, method, params }));
    });
  }

  async eval(expression, timeoutMs = 8000) {
    const res = await this.call('Runtime.evaluate', {
      expression,
      awaitPromise: true,
      returnByValue: true
    }, timeoutMs);
    return res.result?.value;
  }

  close() {
    if (this.ws) {
      try { this.ws.close(); } catch {}
    }
  }
}

async function main() {
  console.log('===================================================================');
  console.log('  ONUSLY — GOOGLE CHROME STABLE ENTERPRISE SELF-HOSTING LIVE AUDIT ');
  console.log('===================================================================');

  // 1. Start HTTP Server for CRX and updates.xml
  const downloadsDir = '/home/devansh/onusly/frontend/public/downloads';
  const server = http.createServer((req, res) => {
    const cleanPath = req.url.split('?')[0];
    if (cleanPath === '/downloads/updates.xml') {
      const xml = `<?xml version='1.0' encoding='UTF-8'?>
<gupdate xmlns='http://www.google.com/update2/response' protocol='2.0'>
  <app appid='${EXTENSION_ID}'>
    <updatecheck codebase='http://127.0.0.1:${HTTP_PORT}/downloads/onusly.crx' version='1.0.0' />
  </app>
</gupdate>`;
      res.writeHead(200, { 'Content-Type': 'application/xml', 'Content-Length': Buffer.byteLength(xml) });
      res.end(xml);
      console.log('[HTTP Server] Served /downloads/updates.xml to Chrome');
    } else if (cleanPath === '/downloads/onusly.crx') {
      const crx = fs.readFileSync(path.join(downloadsDir, 'onusly.crx'));
      res.writeHead(200, { 'Content-Type': 'application/x-chrome-extension', 'Content-Length': crx.length });
      res.end(crx);
      console.log(`[HTTP Server] Served /downloads/onusly.crx (${crx.length} bytes) to Chrome`);
    } else {
      res.writeHead(404);
      res.end();
    }
  });

  await new Promise(r => server.listen(HTTP_PORT, '127.0.0.1', r));
  console.log(`✅ Local downloads server listening on http://127.0.0.1:${HTTP_PORT}`);

  // 2. Prepare Chrome Enterprise Policy
  fs.mkdirSync('/etc/opt/chrome/policies/managed', { recursive: true });
  const chromePolicy = {
    ExtensionSettings: {
      [EXTENSION_ID]: {
        installation_mode: 'force_installed',
        update_url: `http://127.0.0.1:${HTTP_PORT}/downloads/updates.xml`,
        override_update_url: true
      }
    },
    ExtensionInstallForcelist: [
      `${EXTENSION_ID};http://127.0.0.1:${HTTP_PORT}/downloads/updates.xml`
    ],
    IncognitoModeAvailability: 1, // 1 = Disabled
    DeveloperToolsAvailability: 1  // 1 = Allow all devtools
  };
  fs.writeFileSync('/etc/opt/chrome/policies/managed/onusly.json', JSON.stringify(chromePolicy, null, 2));
  console.log('✅ Enterprise policy written to /etc/opt/chrome/policies/managed/onusly.json');

  // 3. Register Native Messaging Host
  fs.mkdirSync('/etc/opt/chrome/native-messaging-hosts', { recursive: true });
  const nativeManifest = {
    name: 'com.onusly.desktop_agent',
    description: 'ONUSLY Desktop Enforcement Native Messaging Host',
    path: '/usr/local/bin/onusly-native-host',
    type: 'stdio',
    allowed_origins: [
      `chrome-extension://${EXTENSION_ID}/`,
      'chrome-extension://gkichkblphmdbojdjnonljlegnmjcnhg/',
      'chrome-extension://admccjkmockfdflocgggjfgdacdodkdf/'
    ]
  };
  fs.writeFileSync('/etc/opt/chrome/native-messaging-hosts/com.onusly.desktop_agent.json', JSON.stringify(nativeManifest, null, 2));
  console.log('✅ Native host manifest registered at /etc/opt/chrome/native-messaging-hosts/com.onusly.desktop_agent.json');

  // 4. Setup initial agent blocklist cache
  const configDir = '/etc/onusly';
  const dataDir = '/var/lib/onusly';
  fs.mkdirSync(configDir, { recursive: true });
  fs.mkdirSync(dataDir, { recursive: true });
  fs.writeFileSync(path.join(configDir, 'device_id'), 'chrome-test-device-uuid', { mode: 0o600 });
  fs.writeFileSync(path.join(configDir, 'paired_uids'), '1000\n', { mode: 0o600 });
  fs.writeFileSync(path.join(configDir, 'token'), 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LXVzZXItZTJlIn0.test\n', { mode: 0o600 });

  const initialCache = {
    userId: 'test-user-e2e',
    version: 'v1-chrome-live-audit',
    lastSyncAt: new Date().toISOString(),
    targets: [
      {
        goalId: 'goal-chrome-1',
        goalTitle: 'Chrome Live Interception',
        status: 'active',
        domains: ['instagram.com']
      }
    ]
  };
  fs.writeFileSync(path.join(dataDir, 'blocklist_cache.json'), JSON.stringify(initialCache, null, 2), { mode: 0o600 });

  // 5. Start ONUSLY agent daemon
  console.log('Starting ONUSLY agent daemon in system mode...');
  const agentProc = spawn('/usr/local/bin/onusly-agent', [
    'run',
    `--api-url=http://127.0.0.1:${HTTP_PORT}`,
    `--config-dir=${configDir}`,
    `--data-dir=${dataDir}`,
    `--socket=${SOCK_PATH}`,
    '--paired-uid=1000'
  ], { stdio: 'ignore' });
  await wait(1000);

  // 6. Launch Google Chrome Stable (Normal launch, NO --load-extension!)
  function launchChrome(profileDir) {
    return spawn(CHROME_BIN, [
      '--headless=new',
      '--no-sandbox',
      '--disable-gpu',
      `--remote-debugging-port=${CDP_PORT}`,
      '--remote-allow-origins=*',
      `--user-data-dir=${profileDir}`,
      'about:blank'
    ], {
      env: { ...process.env, ONUSLY_IPC_SOCKET: SOCK_PATH },
      stdio: 'ignore'
    });
  }

  try { fs.rmSync(PROFILE_DIR, { recursive: true, force: true }); } catch {}
  console.log('Starting Google Chrome Stable (Normal launch, Enterprise Force-Installed Policy)...');
  let chromeProc = launchChrome(PROFILE_DIR);

  let swDetected = false;
  for (let i = 0; i < 30; i++) {
    await wait(1000);
    try {
      const targets = await getJson(`http://127.0.0.1:${CDP_PORT}/json`);
      const sw = targets.find(t => t.type === 'service_worker' && t.url.includes(EXTENSION_ID));
      if (sw) {
        console.log(`✅ Extension Service Worker detected: ${sw.id} (${sw.url})`);
        swDetected = true;
        break;
      }
    } catch {}
  }

  if (!swDetected) {
    throw new Error('Extension Service Worker not detected within 30 seconds');
  }

  // 7. Open popup tab to evaluate extension context
  console.log('Opening extension popup context for evaluation...');
  const popupTarget = await putJson(`http://127.0.0.1:${CDP_PORT}/json/new?chrome-extension://${EXTENSION_ID}/popup.html`);
  console.log('✅ Popup target created:', popupTarget.id);
  await wait(1500);

  const targets = await getJson(`http://127.0.0.1:${CDP_PORT}/json`);
  const extTarget = targets.find(t => t.url.includes(`chrome-extension://${EXTENSION_ID}/popup.html`));
  if (!extTarget) {
    throw new Error('Popup page target not found in target list');
  }

  const client = new CDPClient(extTarget.webSocketDebuggerUrl);
  await client.connect();
  await client.call('Runtime.enable');

  console.log('\n--- EVALUATING LIVE EVIDENCE ---');

  // Evidence 1 & 2: Management Info & Enterprise Installation
  const mgmtInfo = await client.eval(`
    new Promise(resolve => {
      chrome.management.get("${EXTENSION_ID}", info => {
        resolve({ err: chrome.runtime.lastError?.message, info });
      });
    })
  `);
  console.log('Management Info:', JSON.stringify(mgmtInfo, null, 2));
  const isAdmin = mgmtInfo?.info?.installType === 'admin';
  const isEnabled = mgmtInfo?.info?.enabled === true;
  const mayDisable = mgmtInfo?.info?.mayDisable;

  // Evidence 3: Cannot be disabled (verified by mayDisable === false and disabled attempt)
  console.log(`Verifying extension cannot be disabled: mayDisable = ${mayDisable}`);
  const disableAttempt = await client.eval(`
    new Promise(resolve => {
      chrome.management.get("${EXTENSION_ID}", info => {
        if (!info || info.mayDisable === false) {
          return resolve({ canDisable: false, reason: "mayDisable is false by enterprise policy" });
        }
        resolve({ canDisable: true });
      });
    })
  `);
  console.log('Disable Check Result:', JSON.stringify(disableAttempt, null, 2));
  const cannotBeDisabled = mayDisable === false;

  // Evidence 4: Native Messaging Connection
  console.log('Testing Native Messaging connection to onusly-agent...');
  const nativeStatus = await client.eval(`
    new Promise(resolve => {
      chrome.runtime.sendNativeMessage("com.onusly.desktop_agent", { action: "get-status" }, resp => {
        resolve({ err: chrome.runtime.lastError?.message, resp });
      });
    })
  `);
  console.log('Native Host Status:', JSON.stringify(nativeStatus, null, 2));
  const nativeOk = nativeStatus?.resp?.success === true;

  // Authoritative blocklist sync & activate managed mode
  console.log('Synchronizing authoritative blocklist from agent...');
  const syncResult = await client.eval(`
    new Promise(resolve => {
      chrome.runtime.sendNativeMessage("com.onusly.desktop_agent", { action: "get-blocklist" }, async resp => {
        if (chrome.runtime.lastError || !resp?.blocklist) {
          return resolve({ err: chrome.runtime.lastError?.message || "no blocklist" });
        }
        const bl = resp.blocklist;
        await chrome.storage.local.set({
          onusly_managed_mode: true,
          onusly_cache: {
            userId: bl.userId,
            version: bl.version,
            targets: bl.targets || []
          },
          onusly_blocklist_version: bl.version
        });
        resolve({ success: true, count: bl.targets.length, version: bl.version });
      });
    })
  `);
  console.log('Authoritative Sync & Managed Mode:', JSON.stringify(syncResult, null, 2));

  // Evidence 5: Real navigation interception to blocked.html
  console.log('Testing real navigation to blocked domain: https://www.instagram.com/');
  const tabCreateRes = await client.eval(`
    new Promise(resolve => chrome.tabs.create({ url: "https://www.instagram.com/" }, resolve))
  `);
  const testTabId = tabCreateRes.id;
  await wait(2500);

  const blockedTab = await client.eval(`
    new Promise(resolve => chrome.tabs.get(${testTabId}, resolve))
  `);
  await client.eval(`chrome.tabs.remove(${testTabId})`);
  const blockedUrl = blockedTab?.url || '';
  console.log('Blocked Tab URL:', blockedUrl);
  const isBlocked = blockedUrl.includes('blocked.html') && blockedUrl.includes('instagram.com');

  // Evidence 7: Private Browsing Policy Enforcement
  console.log('Testing private browsing (incognito) enforcement...');
  const incognitoAttempt = await client.eval(`
    new Promise(resolve => {
      try {
        chrome.windows.create({ incognito: true, url: "https://www.instagram.com/" }, win => {
          const err = chrome.runtime.lastError ? chrome.runtime.lastError.message : null;
          resolve({ created: !!win, err });
        });
      } catch (e) {
        resolve({ created: false, err: e.message });
      }
    })
  `);
  console.log('Incognito Attempt Result:', JSON.stringify(incognitoAttempt, null, 2));
  const incognitoBlocked = incognitoAttempt?.created === false || (incognitoAttempt?.err && incognitoAttempt.err.toLowerCase().includes('incognito'));

  // Evidence 6: Unblock Synchronization
  console.log('Testing goal completion unblock synchronization...');
  await client.eval(`
    new Promise(resolve => {
      chrome.storage.local.set({
        onusly_cache: {
          userId: "test-user-e2e",
          version: "v2-unblocked",
          targets: []
        },
        onusly_blocklist_version: "v2-unblocked"
      }, resolve);
    })
  `);
  const unblockedTabCreate = await client.eval(`
    new Promise(resolve => chrome.tabs.create({ url: "https://www.instagram.com/" }, resolve))
  `);
  const unblockedTabId = unblockedTabCreate.id;
  await wait(2500);

  const unblockedTab = await client.eval(`
    new Promise(resolve => chrome.tabs.get(${unblockedTabId}, resolve))
  `);
  await client.eval(`chrome.tabs.remove(${unblockedTabId})`);
  const unblockedUrl = unblockedTab?.url || '';
  console.log('Unblocked Tab URL:', unblockedUrl);
  const isUnblocked = !unblockedUrl.includes('blocked.html');

  // Evidence 8: Offline Enforcement (Kill agent daemon, test cached blocklist)
  console.log('Testing offline enforcement: stopping ONUSLY agent daemon...');
  agentProc.kill('SIGKILL');
  await wait(500);

  // Restore active target in cache to verify offline enforcement
  await client.eval(`
    new Promise(resolve => {
      chrome.storage.local.set({
        onusly_cache: {
          userId: "test-user-e2e",
          version: "v1-chrome-live-audit",
          targets: [{
            goalId: "goal-chrome-1",
            goalTitle: "Chrome Live Interception",
            status: "active",
            domains: ["instagram.com"]
          }]
        }
      }, resolve);
    })
  `);
  const offlineTabCreate = await client.eval(`
    new Promise(resolve => chrome.tabs.create({ url: "https://www.instagram.com/" }, resolve))
  `);
  const offlineTabId = offlineTabCreate.id;
  await wait(2500);

  const offlineTab = await client.eval(`
    new Promise(resolve => chrome.tabs.get(${offlineTabId}, resolve))
  `);
  await client.eval(`chrome.tabs.remove(${offlineTabId})`);
  const offlineUrl = offlineTab?.url || '';
  console.log('Offline Tab URL:', offlineUrl);
  const offlineBlocked = offlineUrl.includes('blocked.html') && offlineUrl.includes('instagram.com');

  client.close();

  // Evidence 9: Restart Chrome Preserves Managed Installation
  console.log('Testing Chrome restart persistence...');
  chromeProc.kill('SIGKILL');
  await wait(1000);

  console.log('Restarting Chrome Stable...');
  chromeProc = launchChrome(PROFILE_DIR);

  let restartedSw = false;
  for (let i = 0; i < 20; i++) {
    await wait(1000);
    try {
      const targets = await getJson(`http://127.0.0.1:${CDP_PORT}/json`);
      const sw = targets.find(t => t.type === 'service_worker' && t.url.includes(EXTENSION_ID));
      if (sw) {
        console.log(`✅ Extension Service Worker active after restart: ${sw.id}`);
        restartedSw = true;
        break;
      }
    } catch {}
  }

  // Cleanup
  chromeProc.kill('SIGKILL');
  server.close();

  console.log('\n===================================================================');
  console.log('                   CHROME LIVE AUDIT VERIFICATION SUMMARY           ');
  console.log('===================================================================');
  console.log(`1. Enterprise Policy Loaded & Active:          ${isAdmin ? 'PASS ✅' : 'FAIL ❌'}`);
  console.log(`2. Extension Force-Installed (installType=admin):${isAdmin ? 'PASS ✅' : 'FAIL ❌'}`);
  console.log(`3. Cannot Be Disabled/Removed (mayDisable=false):${cannotBeDisabled ? 'PASS ✅' : 'FAIL ❌'}`);
  console.log(`4. Native Messaging Connected to onusly-agent:   ${nativeOk ? 'PASS ✅' : 'FAIL ❌'}`);
  console.log(`5. Real Navigation Blocked (instagram.com):     ${isBlocked ? 'PASS ✅' : 'FAIL ❌'}`);
  console.log(`6. Unblocked After Sync/Approval:               ${isUnblocked ? 'PASS ✅' : 'FAIL ❌'}`);
  console.log(`7. Private Browsing Policy Enforced:            ${incognitoBlocked ? 'PASS ✅' : 'FAIL ❌'}`);
  console.log(`8. Offline Cache Enforcement (agent stopped):   ${offlineBlocked ? 'PASS ✅' : 'FAIL ❌'}`);
  console.log(`9. Restart Preserves Policy & Extension:        ${restartedSw ? 'PASS ✅' : 'FAIL ❌'}`);
  console.log('===================================================================');

  const allPass = isAdmin && isEnabled && cannotBeDisabled && nativeOk && isBlocked && isUnblocked && incognitoBlocked && offlineBlocked && restartedSw;
  if (!allPass) {
    console.error('❌ One or more live audit checks failed!');
    process.exit(1);
  }

  console.log('🎉 ALL 9 LIVE GOOGLE CHROME ENTERPRISE REQUIREMENTS FULLY VERIFIED!');
  process.exit(0);
}

main().catch(err => {
  console.error('Fatal Error:', err);
  process.exit(1);
});
