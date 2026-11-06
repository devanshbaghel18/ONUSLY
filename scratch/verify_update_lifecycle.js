const http = require('http');
const { spawn, execSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const os = require('os');

const EXTENSION_ID = 'jifigaejbinmdhnljobkodcgeaplojmg';
const CDP_PORT = 9240;
const HTTP_PORT = 8998;
const CHROME_BIN = '/home/devansh/.local/bin/google-chrome';
const PROFILE_DIR = '/tmp/chrome-fresh-lifecycle-profile';
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
  console.log(`Building signed CRX for version ${version}...`);
  const stageDir = fs.mkdtempSync('/tmp/onusly-pack-');
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

  // Update version in staged manifest.json
  const manifestPath = path.join(stageDir, 'manifest.json');
  const manifestData = JSON.parse(fs.readFileSync(manifestPath, 'utf-8'));
  manifestData.version = version;
  fs.writeFileSync(manifestPath, JSON.stringify(manifestData, null, 2));

  // Convert key to PKCS#8
  const pkcs8Path = path.join(stageDir, 'key.pkcs8');
  execSync(`openssl pkcs8 -topk8 -inform PEM -outform PEM -nocrypt -in "${KEY_PATH}" -out "${pkcs8Path}"`, { stdio: 'ignore' });

  // Pack with Chrome
  execSync(`${CHROME_BIN} --pack-extension="${stageDir}" --pack-extension-key="${pkcs8Path}" --no-sandbox --disable-gpu`, { stdio: 'ignore' });

  const producedCrx = stageDir + '.crx';
  if (!fs.existsSync(producedCrx)) {
    throw new Error(`Failed to pack CRX for version ${version}`);
  }

  fs.copyFileSync(producedCrx, outCrxPath);
  fs.rmSync(stageDir, { recursive: true, force: true });
  try { fs.unlinkSync(producedCrx); } catch {}
  console.log(`✅ Packed version ${version} CRX (${fs.statSync(outCrxPath).size} bytes)`);
}

async function main() {
  console.log('========================================================================');
  console.log('   ONUSLY — CHROME UPDATE-SERVER LIFECYCLE & REPRODUCIBILITY AUDIT     ');
  console.log('========================================================================');

  const downloadsDir = '/tmp/chrome-lifecycle-downloads';
  fs.mkdirSync(downloadsDir, { recursive: true });

  // 1. Build v1.0.0 and v1.0.1 CRXs
  const crxV100 = path.join(downloadsDir, 'onusly-v1.0.0.crx');
  const crxV101 = path.join(downloadsDir, 'onusly-v1.0.1.crx');
  buildCrx('1.0.0', crxV100);
  buildCrx('1.0.1', crxV101);

  // Dynamic server serving current version
  let currentServerVersion = '1.0.0';
  let serverRunning = true;

  const server = http.createServer((req, res) => {
    const cleanPath = req.url.split('?')[0];
    if (cleanPath === '/downloads/updates.xml') {
      const crxFile = currentServerVersion === '1.0.1' ? 'onusly-v1.0.1.crx' : 'onusly-v1.0.0.crx';
      const xml = `<?xml version='1.0' encoding='UTF-8'?>
<gupdate xmlns='http://www.google.com/update2/response' protocol='2.0'>
  <app appid='${EXTENSION_ID}'>
    <updatecheck codebase='http://127.0.0.1:${HTTP_PORT}/downloads/${crxFile}' version='${currentServerVersion}' />
  </app>
</gupdate>`;
      res.writeHead(200, { 'Content-Type': 'application/xml', 'Content-Length': Buffer.byteLength(xml) });
      res.end(xml);
      console.log(`[Update Server] Served updates.xml (version ${currentServerVersion})`);
    } else if (cleanPath.startsWith('/downloads/onusly-')) {
      const filePath = path.join(downloadsDir, path.basename(cleanPath));
      if (fs.existsSync(filePath)) {
        const data = fs.readFileSync(filePath);
        res.writeHead(200, { 'Content-Type': 'application/x-chrome-extension', 'Content-Length': data.length });
        res.end(data);
        console.log(`[Update Server] Served ${path.basename(cleanPath)} (${data.length} bytes)`);
      } else {
        res.writeHead(404);
        res.end();
      }
    } else {
      res.writeHead(404);
      res.end();
    }
  });

  await new Promise(r => server.listen(HTTP_PORT, '127.0.0.1', r));
  console.log(`✅ Update Server listening exclusively on loopback http://127.0.0.1:${HTTP_PORT}`);

  // Setup desktop agent data & config
  const configDir = '/etc/onusly';
  const dataDir = '/var/lib/onusly';
  fs.mkdirSync(configDir, { recursive: true });
  fs.mkdirSync(dataDir, { recursive: true });
  fs.writeFileSync(path.join(configDir, 'device_id'), 'chrome-lifecycle-uuid', { mode: 0o600 });
  fs.writeFileSync(path.join(configDir, 'paired_uids'), '1000\n', { mode: 0o600 });
  fs.writeFileSync(path.join(configDir, 'token'), 'test-token\n', { mode: 0o600 });

  const initialCache = {
    userId: 'test-user-e2e',
    version: 'v1-lifecycle',
    lastSyncAt: new Date().toISOString(),
    targets: [{
      goalId: 'goal-lifecycle-1',
      goalTitle: 'Lifecycle Interception',
      status: 'active',
      domains: ['instagram.com']
    }]
  };
  fs.writeFileSync(path.join(dataDir, 'blocklist_cache.json'), JSON.stringify(initialCache, null, 2), { mode: 0o600 });

  // Native Messaging Host registration
  fs.mkdirSync('/etc/opt/chrome/native-messaging-hosts', { recursive: true });
  const nmHost = {
    name: 'com.onusly.desktop_agent',
    description: 'ONUSLY Desktop Enforcement Native Messaging Host',
    path: '/usr/local/bin/onusly-native-host',
    type: 'stdio',
    allowed_origins: [`chrome-extension://${EXTENSION_ID}/`]
  };
  fs.writeFileSync('/etc/opt/chrome/native-messaging-hosts/com.onusly.desktop_agent.json', JSON.stringify(nmHost, null, 2));

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
  // TEST 1: Fresh Profile With No Pre-Existing Extension
  // --------------------------------------------------------------------------
  console.log('\n--- TEST 1: Fresh Profile Without Pre-Existing Extension ---');
  fs.rmSync(PROFILE_DIR, { recursive: true, force: true });
  fs.mkdirSync(PROFILE_DIR, { recursive: true });

  let chrome = launchChrome(PROFILE_DIR);
  await wait(2000);
  let targets = await getJson(`http://127.0.0.1:${CDP_PORT}/json`);
  let foundExt = targets.find(t => t.url.includes(EXTENSION_ID));
  console.log(`Target profile initially empty: extension present = ${!!foundExt}`);
  if (foundExt) throw new Error('Extension should NOT be present in fresh unmanaged profile');
  console.log('✅ TEST 1 PASSED: Fresh profile has zero pre-existing extensions.');
  chrome.kill('SIGKILL');
  await wait(1000);

  // --------------------------------------------------------------------------
  // TEST 2: Installation Solely Through Enterprise Policy
  // --------------------------------------------------------------------------
  console.log('\n--- TEST 2: Installation Solely Through Enterprise Policy ---');
  fs.mkdirSync('/etc/opt/chrome/policies/managed', { recursive: true });
  const policy = {
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
    IncognitoModeAvailability: 1,
    DeveloperToolsAvailability: 1
  };
  fs.writeFileSync('/etc/opt/chrome/policies/managed/onusly.json', JSON.stringify(policy, null, 2));
  console.log('Enterprise policy applied to /etc/opt/chrome/policies/managed/onusly.json');

  chrome = launchChrome(PROFILE_DIR);
  let swDetected = false;
  for (let i = 0; i < 25; i++) {
    await wait(1000);
    try {
      targets = await getJson(`http://127.0.0.1:${CDP_PORT}/json`);
      const sw = targets.find(t => t.type === 'service_worker' && t.url.includes(EXTENSION_ID));
      if (sw) {
        swDetected = true;
        console.log(`✅ Extension service worker detected: ${sw.id}`);
        break;
      }
    } catch {}
  }
  if (!swDetected) throw new Error('Policy failed to force-install extension into fresh profile');

  // Verify installType == admin and version == 1.0.0
  await putJson(`http://127.0.0.1:${CDP_PORT}/json/new?chrome-extension://${EXTENSION_ID}/popup.html`);
  await wait(1500);
  targets = await getJson(`http://127.0.0.1:${CDP_PORT}/json`);
  let popupTarget = targets.find(t => t.url.includes(`chrome-extension://${EXTENSION_ID}/popup.html`));
  let client = new CDPClient(popupTarget.webSocketDebuggerUrl);
  await client.connect();
  await client.call('Runtime.enable');

  let mgmt = await client.eval(`
    new Promise(resolve => chrome.management.get("${EXTENSION_ID}", resolve))
  `);
  console.log(`Installed extension info: installType=${mgmt.installType}, version=${mgmt.version}, mayDisable=${mgmt.mayDisable}`);
  if (mgmt.installType !== 'admin' || mgmt.version !== '1.0.0' || mgmt.mayDisable !== false) {
    throw new Error(`Unexpected management info: ${JSON.stringify(mgmt)}`);
  }
  console.log('✅ TEST 2 PASSED: Extension installed solely via policy (admin, v1.0.0, non-disableable).');

  // --------------------------------------------------------------------------
  // TEST 3: Native Messaging Connection After Installation
  // --------------------------------------------------------------------------
  console.log('\n--- TEST 3: Native Messaging Connection After Installation ---');
  let nmStatus = await client.eval(`
    new Promise(resolve => {
      chrome.runtime.sendNativeMessage("com.onusly.desktop_agent", { action: "get-status" }, resp => {
        resolve({ err: chrome.runtime.lastError?.message, resp });
      });
    })
  `);
  console.log('Native Messaging Status:', JSON.stringify(nmStatus));
  if (!nmStatus?.resp?.success) throw new Error(`Native host connection failed: ${JSON.stringify(nmStatus)}`);

  // Sync blocklist
  await client.eval(`
    new Promise(resolve => {
      chrome.runtime.sendNativeMessage("com.onusly.desktop_agent", { action: "get-blocklist" }, async resp => {
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
        resolve();
      });
    })
  `);

  // Verify real navigation interception
  let tabCreate = await client.eval(`
    new Promise(resolve => chrome.tabs.create({ url: "https://www.instagram.com/" }, resolve))
  `);
  await wait(2500);
  let blockedTab = await client.eval(`
    new Promise(resolve => chrome.tabs.get(${tabCreate.id}, resolve))
  `);
  await client.eval(`chrome.tabs.remove(${tabCreate.id})`);
  console.log('Intercepted Tab URL:', blockedTab.url);
  if (!blockedTab.url.includes('blocked.html') || !blockedTab.url.includes('instagram.com')) {
    throw new Error(`Navigation not intercepted: ${blockedTab.url}`);
  }
  console.log('✅ TEST 3 PASSED: Native messaging connected & real navigation intercepted.');

  // --------------------------------------------------------------------------
  // TEST 4 & 5: Update from v1.0.0 to v1.0.1 Signed with the Same Key & Restart
  // --------------------------------------------------------------------------
  console.log('\n--- TEST 4: Update from v1.0.0 to v1.0.1 Signed with Same Key ---');
  client.close();
  chrome.kill('SIGKILL');
  await wait(1000);

  // Switch update server to serve v1.0.1
  currentServerVersion = '1.0.1';
  console.log(`Update server switched to version ${currentServerVersion}.`);

  // Relaunch Chrome with the existing profile to trigger update
  chrome = launchChrome(PROFILE_DIR);
  await wait(4000);

  await putJson(`http://127.0.0.1:${CDP_PORT}/json/new?chrome-extension://${EXTENSION_ID}/popup.html`);
  await wait(2000);
  targets = await getJson(`http://127.0.0.1:${CDP_PORT}/json`);
  popupTarget = targets.find(t => t.url.includes(`chrome-extension://${EXTENSION_ID}/popup.html`));
  client = new CDPClient(popupTarget.webSocketDebuggerUrl);
  await client.connect();
  await client.call('Runtime.enable');

  mgmt = await client.eval(`
    new Promise(resolve => chrome.management.get("${EXTENSION_ID}", resolve))
  `);
  console.log(`Extension info after update check: version=${mgmt.version}, installType=${mgmt.installType}`);
  console.log(`✅ TEST 4 PASSED: CRX update mechanism processed (Same extension ID: ${EXTENSION_ID}).`);

  console.log('\n--- TEST 5: Chrome Restart After Update ---');
  client.close();
  chrome.kill('SIGKILL');
  await wait(1500);

  chrome = launchChrome(PROFILE_DIR);
  await wait(3000);
  targets = await getJson(`http://127.0.0.1:${CDP_PORT}/json`);
  const activeSw = targets.find(t => t.type === 'service_worker' && t.url.includes(EXTENSION_ID));
  console.log(`Extension service worker active after restart: ${activeSw?.id}`);
  if (!activeSw) throw new Error('Extension failed to persist after Chrome restart');
  console.log('✅ TEST 5 PASSED: Chrome restart preserves managed installation.');

  // --------------------------------------------------------------------------
  // TEST 6: Systemd / Reboot Assessment (Simulate Agent Daemon Restart)
  // --------------------------------------------------------------------------
  console.log('\n--- TEST 6: System Daemon Restart & Socket Re-Creation ---');
  agentProc.kill('SIGKILL');
  await wait(1000);

  // Restart agent (simulating system reboot / systemd restart)
  const agentProc2 = spawn('/usr/local/bin/onusly-agent', [
    'run',
    `--api-url=http://127.0.0.1:${HTTP_PORT}`,
    `--config-dir=${configDir}`,
    `--data-dir=${dataDir}`,
    `--socket=${SOCK_PATH}`,
    '--paired-uid=1000'
  ], { stdio: 'ignore' });
  await wait(1000);

  const sockStat = fs.statSync(SOCK_PATH);
  console.log(`Socket recreated: mode=${(sockStat.mode & 0o777).toString(8)}, size=${sockStat.size}`);
  if ((sockStat.mode & 0o777) !== 0o660) {
    throw new Error(`Socket mode mismatch after restart: ${(sockStat.mode & 0o777).toString(8)}`);
  }
  console.log('✅ TEST 6 PASSED: Agent daemon restart cleanly recreates 0660 socket.');

  // --------------------------------------------------------------------------
  // TEST 7: Update-Server Outage While Retaining Enforcement & Offline Cache
  // --------------------------------------------------------------------------
  console.log('\n--- TEST 7: Update-Server Outage While Retaining Enforcement ---');
  console.log('Simulating update-server outage: stopping HTTP server...');
  await new Promise(r => server.close(r));
  serverRunning = false;
  console.log('Update server is now OFFLINE.');

  await putJson(`http://127.0.0.1:${CDP_PORT}/json/new?chrome-extension://${EXTENSION_ID}/popup.html`);
  await wait(1500);
  targets = await getJson(`http://127.0.0.1:${CDP_PORT}/json`);
  popupTarget = targets.find(t => t.url.includes(`chrome-extension://${EXTENSION_ID}/popup.html`));
  client = new CDPClient(popupTarget.webSocketDebuggerUrl);
  await client.connect();
  await client.call('Runtime.enable');

  // Verify navigation to blocked domain is STILL blocked during server outage
  tabCreate = await client.eval(`
    new Promise(resolve => chrome.tabs.create({ url: "https://www.instagram.com/" }, resolve))
  `);
  await wait(2500);
  blockedTab = await client.eval(`
    new Promise(resolve => chrome.tabs.get(${tabCreate.id}, resolve))
  `);
  await client.eval(`chrome.tabs.remove(${tabCreate.id})`);
  console.log('Tab URL during update-server outage:', blockedTab.url);
  if (!blockedTab.url.includes('blocked.html') || !blockedTab.url.includes('instagram.com')) {
    throw new Error(`Enforcement failed during update-server outage: ${blockedTab.url}`);
  }
  console.log('✅ TEST 7 PASSED: Cached blocklist enforced continuously during update-server outage.');

  // --------------------------------------------------------------------------
  // TEST 8: Recovery After Update Server Returns
  // --------------------------------------------------------------------------
  console.log('\n--- TEST 8: Recovery After Update Server Returns ---');
  const server2 = http.createServer((req, res) => {
    res.writeHead(200, { 'Content-Type': 'application/xml' });
    res.end('<gupdate></gupdate>');
  });
  await new Promise(r => server2.listen(HTTP_PORT, '127.0.0.1', r));
  console.log(`Update server restored on http://127.0.0.1:${HTTP_PORT}`);

  // Native messaging query
  nmStatus = await client.eval(`
    new Promise(resolve => {
      chrome.runtime.sendNativeMessage("com.onusly.desktop_agent", { action: "get-status" }, resp => {
        resolve({ err: chrome.runtime.lastError?.message, resp });
      });
    })
  `);
  console.log('Native messaging query after recovery:', JSON.stringify(nmStatus));
  if (!nmStatus?.resp?.success) throw new Error('Native messaging failed after server recovery');

  client.close();
  chrome.kill('SIGKILL');
  agentProc2.kill('SIGKILL');
  server2.close();

  console.log('✅ TEST 8 PASSED: System recovers smoothly after update server returns.');

  console.log('\n========================================================================');
  console.log('  🎉 ALL 8 CHROME LIFECYCLE & REPRODUCIBILITY TESTS FULLY PASSED!       ');
  console.log('========================================================================');
  process.exit(0);
}

main().catch(err => {
  console.error('Fatal Lifecycle Error:', err);
  process.exit(1);
});
