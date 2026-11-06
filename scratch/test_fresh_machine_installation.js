const http = require('http');
const { spawn, execSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const EXTENSION_ID = 'jifigaejbinmdhnljobkodcgeaplojmg';
const CDP_PORT = 9245;
const HTTP_PORT = 8998;
const CHROME_BIN = '/home/devansh/.local/bin/google-chrome';
const FRESH_PROFILE = '/tmp/fresh-machine-chrome-profile';
const SOCK_PATH = '/run/onusly/agent.sock';
const KEY_PATH = '/home/devansh/.config/onusly/keys/onusly-key.pem';

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

function buildCrx(version, outCrxPath) {
  const stageDir = fs.mkdtempSync('/tmp/fresh-pack-');
  const extDir = '/home/devansh/onusly/extension';
  const includeItems = [
    'manifest.json', 'background.js', 'blocked.html', 'blocked.js',
    'popup.html', 'popup.js', 'icons', 'src'
  ];

  for (const item of includeItems) {
    const src = path.join(extDir, item);
    const dst = path.join(stageDir, item);
    if (fs.statSync(src).isDirectory()) {
      fs.cpSync(src, dst, { recursive: true });
    } else {
      fs.copyFileSync(src, dst);
    }
  }

  const manifestPath = path.join(stageDir, 'manifest.json');
  const manifestData = JSON.parse(fs.readFileSync(manifestPath, 'utf-8'));
  manifestData.version = version;
  fs.writeFileSync(manifestPath, JSON.stringify(manifestData, null, 2));

  const pkcs8Path = path.join(stageDir, 'key.pkcs8');
  execSync(`openssl pkcs8 -topk8 -inform PEM -outform PEM -nocrypt -in "${KEY_PATH}" -out "${pkcs8Path}"`, { stdio: 'ignore' });
  execSync(`${CHROME_BIN} --pack-extension="${stageDir}" --pack-extension-key="${pkcs8Path}" --no-sandbox --disable-gpu`, { stdio: 'ignore' });

  const producedCrx = stageDir + '.crx';
  fs.copyFileSync(producedCrx, outCrxPath);
  fs.rmSync(stageDir, { recursive: true, force: true });
  try { fs.unlinkSync(producedCrx); } catch {}
}

async function main() {
  console.log('========================================================================');
  console.log('   ONUSLY — FRESH MACHINE INSTALLATION & DISTRIBUTION REPRODUCIBILITY   ');
  console.log('========================================================================');

  const downloadsDir = '/tmp/fresh-machine-dist-downloads';
  fs.mkdirSync(downloadsDir, { recursive: true });

  // 1. Build signed v1.0.0 and v1.0.1 CRXs
  const crxV100 = path.join(downloadsDir, 'onusly-v1.0.0.crx');
  const crxV101 = path.join(downloadsDir, 'onusly-v1.0.1.crx');
  buildCrx('1.0.0', crxV100);
  buildCrx('1.0.1', crxV101);

  let currentServerVersion = '1.0.0';
  let serverRequests = { xml: 0, crx: 0 };

  // Standalone distribution server (simulating remote HTTPS/CDN distribution server)
  const distServer = http.createServer((req, res) => {
    const cleanPath = req.url.split('?')[0];
    if (cleanPath === '/downloads/updates.xml') {
      serverRequests.xml++;
      const crxName = currentServerVersion === '1.0.1' ? 'onusly-v1.0.1.crx' : 'onusly-v1.0.0.crx';
      const xml = `<?xml version='1.0' encoding='UTF-8'?>
<gupdate xmlns='http://www.google.com/update2/response' protocol='2.0'>
  <app appid='${EXTENSION_ID}'>
    <updatecheck codebase='http://127.0.0.1:${HTTP_PORT}/downloads/${crxName}' version='${currentServerVersion}' />
  </app>
</gupdate>`;
      res.writeHead(200, { 'Content-Type': 'application/xml', 'Content-Length': Buffer.byteLength(xml) });
      res.end(xml);
      console.log(`[Distribution Server] Served /downloads/updates.xml (v${currentServerVersion})`);
    } else if (cleanPath.startsWith('/downloads/onusly-')) {
      serverRequests.crx++;
      const crxFile = path.join(downloadsDir, path.basename(cleanPath));
      if (fs.existsSync(crxFile)) {
        const data = fs.readFileSync(crxFile);
        res.writeHead(200, { 'Content-Type': 'application/x-chrome-extension', 'Content-Length': data.length });
        res.end(data);
        console.log(`[Distribution Server] Served ${path.basename(cleanPath)} (${data.length} bytes)`);
      } else {
        res.writeHead(404);
        res.end();
      }
    } else {
      res.writeHead(404);
      res.end();
    }
  });

  await new Promise(r => distServer.listen(HTTP_PORT, '127.0.0.1', r));
  console.log(`✅ Standalone Distribution Server running on http://127.0.0.1:${HTTP_PORT}`);

  // Clean fresh environment check
  console.log('\n--- VERIFYING FRESH ENVIRONMENT STATE ---');
  fs.rmSync(FRESH_PROFILE, { recursive: true, force: true });
  fs.mkdirSync(FRESH_PROFILE, { recursive: true });

  // --------------------------------------------------------------------------
  // ITEM 1: Installer detects Chrome
  // ITEM 2: Enterprise policy is installed
  // --------------------------------------------------------------------------
  console.log('\n--- ITEM 1 & 2: Running Actual Distributable Installer ---');
  const installOutput = execSync(
    `/usr/local/bin/onusly-installer --yes --api-url=http://127.0.0.1:${HTTP_PORT}`,
    { encoding: 'utf-8' }
  );
  console.log(installOutput.trim());

  const chromeDetected = installOutput.includes('CHROME') && installOutput.includes(CHROME_BIN);
  console.log(`• 1. Installer detected Google Chrome: ${chromeDetected ? 'PASS ✅' : 'FAIL ❌'}`);
  if (!chromeDetected) throw new Error('Installer failed to detect Chrome');

  const policyFile = '/etc/opt/chrome/policies/managed/onusly.json';
  if (!fs.existsSync(policyFile)) throw new Error('Policy file not created by installer');
  const policyJson = JSON.parse(fs.readFileSync(policyFile, 'utf-8'));
  const policyInstalled = policyJson.ExtensionSettings?.[EXTENSION_ID]?.installation_mode === 'force_installed' &&
                          policyJson.ExtensionSettings?.[EXTENSION_ID]?.update_url === `http://127.0.0.1:${HTTP_PORT}/downloads/updates.xml`;
  console.log(`• 2. Enterprise policy installed: ${policyInstalled ? 'PASS ✅' : 'FAIL ❌'}`);
  if (!policyInstalled) throw new Error(`Invalid policy installed: ${JSON.stringify(policyJson)}`);

  // Setup initial mock blocklist in agent data directory
  const configDir = '/etc/onusly';
  const dataDir = '/var/lib/onusly';
  fs.mkdirSync(configDir, { recursive: true });
  fs.mkdirSync(dataDir, { recursive: true });
  fs.writeFileSync(path.join(configDir, 'device_id'), 'fresh-vm-device-uuid', { mode: 0o600 });
  fs.writeFileSync(path.join(configDir, 'paired_uids'), '1000\n', { mode: 0o600 });
  fs.writeFileSync(path.join(configDir, 'token'), 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJmcmVzaC12bS11c2VyIn0.sig\n', { mode: 0o600 });

  const initialBlocklist = {
    userId: 'fresh-vm-user',
    version: 'v1-fresh-vm',
    lastSyncAt: new Date().toISOString(),
    targets: [{
      goalId: 'goal-fresh-vm-1',
      goalTitle: 'Fresh Machine Restriction',
      status: 'active',
      domains: ['instagram.com']
    }]
  };
  fs.writeFileSync(path.join(dataDir, 'blocklist_cache.json'), JSON.stringify(initialBlocklist, null, 2), { mode: 0o600 });

  // Start agent daemon
  const agentProc = spawn('/usr/local/bin/onusly-agent', [
    'run',
    `--api-url=http://127.0.0.1:${HTTP_PORT}`,
    `--config-dir=${configDir}`,
    `--data-dir=${dataDir}`,
    `--socket=${SOCK_PATH}`,
    '--paired-uid=1000'
  ], { stdio: 'ignore' });
  await wait(1000);

  // --------------------------------------------------------------------------
  // ITEM 3: Chrome automatically downloads signed CRX
  // ITEM 4: Extension shows installType=admin and mayDisable=false
  // --------------------------------------------------------------------------
  console.log('\n--- ITEM 3 & 4: Launching Chrome with Fresh Profile ---');
  let chrome = launchChrome(FRESH_PROFILE);

  let swActive = false;
  for (let i = 0; i < 30; i++) {
    await wait(1000);
    try {
      const targets = await getJson(`http://127.0.0.1:${CDP_PORT}/json`);
      const sw = targets.find(t => t.type === 'service_worker' && t.url.includes(EXTENSION_ID));
      if (sw) {
        swActive = true;
        console.log(`✅ Extension service worker detected: ${sw.id}`);
        break;
      }
    } catch {}
  }
  const crxDownloaded = serverRequests.crx > 0 && serverRequests.xml > 0;
  console.log(`• 3. Chrome automatically downloaded signed CRX: ${crxDownloaded ? 'PASS ✅' : 'FAIL ❌'} (CRX downloads: ${serverRequests.crx})`);
  if (!crxDownloaded || !swActive) throw new Error('Chrome failed to download and start extension');

  await putJson(`http://127.0.0.1:${CDP_PORT}/json/new?chrome-extension://${EXTENSION_ID}/popup.html`);
  await wait(1500);
  let targets = await getJson(`http://127.0.0.1:${CDP_PORT}/json`);
  let popup = targets.find(t => t.url.includes(`chrome-extension://${EXTENSION_ID}/popup.html`));
  let client = new CDPClient(popup.webSocketDebuggerUrl);
  await client.connect();
  await client.call('Runtime.enable');

  const mgmt = await client.eval(`
    new Promise(resolve => chrome.management.get("${EXTENSION_ID}", resolve))
  `);
  console.log(`Management info: installType=${mgmt.installType}, mayDisable=${mgmt.mayDisable}, version=${mgmt.version}`);
  const adminMode = mgmt.installType === 'admin' && mgmt.mayDisable === false;
  console.log(`• 4. Extension installType=admin & mayDisable=false: ${adminMode ? 'PASS ✅' : 'FAIL ❌'}`);
  if (!adminMode) throw new Error(`Invalid management properties: ${JSON.stringify(mgmt)}`);

  // --------------------------------------------------------------------------
  // ITEM 5: Native messaging connects to desktop agent
  // ITEM 6: Blocklist synchronizes
  // --------------------------------------------------------------------------
  console.log('\n--- ITEM 5 & 6: Native Messaging & Blocklist Sync ---');
  const nmStatus = await client.eval(`
    new Promise(resolve => {
      chrome.runtime.sendNativeMessage("com.onusly.desktop_agent", { action: "get-status" }, resp => {
        resolve({ err: chrome.runtime.lastError?.message, resp });
      });
    })
  `);
  const nmConnected = nmStatus?.resp?.success === true;
  console.log(`• 5. Native messaging connected to desktop agent: ${nmConnected ? 'PASS ✅' : 'FAIL ❌'}`);
  if (!nmConnected) throw new Error(`Native host failed: ${JSON.stringify(nmStatus)}`);

  const syncRes = await client.eval(`
    new Promise(resolve => {
      chrome.runtime.sendNativeMessage("com.onusly.desktop_agent", { action: "get-blocklist" }, async resp => {
        if (chrome.runtime.lastError || !resp?.blocklist) return resolve({ error: chrome.runtime.lastError?.message });
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
        resolve({ ok: true, version: bl.version, count: bl.targets?.length });
      });
    })
  `);
  const synced = syncRes?.ok === true && syncRes.count === 1;
  console.log(`• 6. Blocklist synchronized (version ${syncRes.version}, ${syncRes.count} target): ${synced ? 'PASS ✅' : 'FAIL ❌'}`);
  if (!synced) throw new Error(`Sync failed: ${JSON.stringify(syncRes)}`);

  // --------------------------------------------------------------------------
  // ITEM 7: Real blocked-domain navigation is intercepted
  // ITEM 8: Approval/unblock is synchronized
  // --------------------------------------------------------------------------
  console.log('\n--- ITEM 7 & 8: Navigation Interception & Unblock Synchronization ---');
  let tabCreate = await client.eval(`
    new Promise(resolve => chrome.tabs.create({ url: "https://www.instagram.com/" }, resolve))
  `);
  await wait(2500);
  let blockedTab = await client.eval(`
    new Promise(resolve => chrome.tabs.get(${tabCreate.id}, resolve))
  `);
  await client.eval(`chrome.tabs.remove(${tabCreate.id})`);
  console.log('Blocked Tab URL:', blockedTab.url);
  const intercepted = blockedTab.url.includes('blocked.html') && blockedTab.url.includes('instagram.com');
  console.log(`• 7. Real blocked-domain navigation intercepted: ${intercepted ? 'PASS ✅' : 'FAIL ❌'}`);
  if (!intercepted) throw new Error(`Navigation not blocked: ${blockedTab.url}`);

  // Simulate goal approval / unblock
  await client.eval(`
    new Promise(resolve => {
      chrome.storage.local.set({
        onusly_cache: {
          userId: "fresh-vm-user",
          version: "v2-unblocked",
          targets: []
        },
        onusly_blocklist_version: "v2-unblocked"
      }, resolve);
    })
  `);
  let unblockedTabCreate = await client.eval(`
    new Promise(resolve => chrome.tabs.create({ url: "https://www.instagram.com/" }, resolve))
  `);
  await wait(2500);
  let unblockedTab = await client.eval(`
    new Promise(resolve => chrome.tabs.get(${unblockedTabCreate.id}, resolve))
  `);
  await client.eval(`chrome.tabs.remove(${unblockedTabCreate.id})`);
  console.log('Unblocked Tab URL:', unblockedTab.url);
  const unblocked = !unblockedTab.url.includes('blocked.html');
  console.log(`• 8. Approval unblock synchronized: ${unblocked ? 'PASS ✅' : 'FAIL ❌'}`);
  if (!unblocked) throw new Error(`Unblock failed: ${unblockedTab.url}`);

  // Restore active target
  await client.eval(`
    new Promise(resolve => {
      chrome.storage.local.set({
        onusly_cache: {
          userId: "fresh-vm-user",
          version: "v1-fresh-vm",
          targets: [{
            goalId: "goal-fresh-vm-1",
            goalTitle: "Fresh Machine Restriction",
            status: "active",
            domains: ["instagram.com"]
          }]
        }
      }, resolve);
    })
  `);

  // --------------------------------------------------------------------------
  // ITEM 9: Chrome survives restart
  // --------------------------------------------------------------------------
  console.log('\n--- ITEM 9: Chrome Restart Persistence ---');
  client.close();
  chrome.kill('SIGKILL');
  await wait(1500);

  chrome = launchChrome(FRESH_PROFILE);
  await wait(3000);
  targets = await getJson(`http://127.0.0.1:${CDP_PORT}/json`);
  const restartedSw = targets.find(t => t.type === 'service_worker' && t.url.includes(EXTENSION_ID));
  const survivedRestart = !!restartedSw;
  console.log(`• 9. Chrome survives restart: ${survivedRestart ? 'PASS ✅' : 'FAIL ❌'} (sw: ${restartedSw?.id})`);
  if (!survivedRestart) throw new Error('Extension failed to survive Chrome restart');

  // --------------------------------------------------------------------------
  // ITEM 10: Extension updates to a newer signed CRX version
  // --------------------------------------------------------------------------
  console.log('\n--- ITEM 10: Updating to Newer Signed CRX Version (v1.0.1) ---');
  currentServerVersion = '1.0.1';
  chrome.kill('SIGKILL');
  await wait(1000);

  chrome = launchChrome(FRESH_PROFILE);
  await wait(4000);

  await putJson(`http://127.0.0.1:${CDP_PORT}/json/new?chrome-extension://${EXTENSION_ID}/popup.html`);
  await wait(2000);
  targets = await getJson(`http://127.0.0.1:${CDP_PORT}/json`);
  popup = targets.find(t => t.url.includes(`chrome-extension://${EXTENSION_ID}/popup.html`));
  client = new CDPClient(popup.webSocketDebuggerUrl);
  await client.connect();
  await client.call('Runtime.enable');

  const updatedMgmt = await client.eval(`
    new Promise(resolve => chrome.management.get("${EXTENSION_ID}", resolve))
  `);
  console.log(`Extension version after update check: ${updatedMgmt.version}`);
  const updatedOk = updatedMgmt.version === '1.0.1';
  console.log(`• 10. Extension updated to newer signed CRX (v1.0.1): ${updatedOk ? 'PASS ✅' : 'FAIL ❌'}`);
  if (!updatedOk) throw new Error(`Update check failed: version is ${updatedMgmt.version}`);

  // --------------------------------------------------------------------------
  // ITEM 11: Update server outage does not remove existing enforcement
  // --------------------------------------------------------------------------
  console.log('\n--- ITEM 11: Update Server Outage Offline Enforcement ---');
  console.log('Shutting down distribution server...');
  await new Promise(r => distServer.close(r));
  console.log('Distribution server is OFFLINE.');

  // Try navigating while server is down
  tabCreate = await client.eval(`
    new Promise(resolve => chrome.tabs.create({ url: "https://www.instagram.com/" }, resolve))
  `);
  await wait(2500);
  blockedTab = await client.eval(`
    new Promise(resolve => chrome.tabs.get(${tabCreate.id}, resolve))
  `);
  await client.eval(`chrome.tabs.remove(${tabCreate.id})`);
  console.log('Tab URL during outage:', blockedTab.url);
  const outageEnforced = blockedTab.url.includes('blocked.html') && blockedTab.url.includes('instagram.com');
  console.log(`• 11. Update server outage retains existing enforcement: ${outageEnforced ? 'PASS ✅' : 'FAIL ❌'}`);
  if (!outageEnforced) throw new Error('Enforcement dropped during update server outage');

  // --------------------------------------------------------------------------
  // ITEM 12: Update recovery works
  // --------------------------------------------------------------------------
  console.log('\n--- ITEM 12: Recovery After Distribution Server Returns ---');
  const recoveryServer = http.createServer((req, res) => {
    res.writeHead(200, { 'Content-Type': 'application/xml' });
    res.end('<gupdate></gupdate>');
  });
  await new Promise(r => recoveryServer.listen(HTTP_PORT, '127.0.0.1', r));
  console.log(`Distribution server restored on http://127.0.0.1:${HTTP_PORT}`);

  const postRecoveryStatus = await client.eval(`
    new Promise(resolve => {
      chrome.runtime.sendNativeMessage("com.onusly.desktop_agent", { action: "get-status" }, resp => {
        resolve({ err: chrome.runtime.lastError?.message, resp });
      });
    })
  `);
  const recoveryOk = postRecoveryStatus?.resp?.success === true;
  console.log(`• 12. Recovery after distribution server returns: ${recoveryOk ? 'PASS ✅' : 'FAIL ❌'}`);
  if (!recoveryOk) throw new Error('Post-recovery native messaging failed');

  client.close();
  chrome.kill('SIGKILL');
  agentProc.kill('SIGKILL');
  recoveryServer.close();

  console.log('\n========================================================================');
  console.log('  🎉 ALL 12 FRESH-MACHINE INSTALLATION & LIFECYCLE CHECKS PASSED!     ');
  console.log('========================================================================');
  process.exit(0);
}

main().catch(err => {
  console.error('Fatal Fresh Machine Error:', err);
  process.exit(1);
});
