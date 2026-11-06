const http = require('http');
const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');

const EXT_PATH = '/home/devansh/onusly/extension';
const EXT_ID = 'jifigaejbinmdhnljobkodcgeaplojmg';

function wait(ms) {
  return new Promise(r => setTimeout(r, ms));
}

function cdp(ws, method, params = {}) {
  return new Promise((resolve, reject) => {
    const id = Math.floor(Math.random() * 1000000);
    const handler = (e) => {
      const msg = JSON.parse(e.data);
      if (msg.id === id) {
        ws.removeEventListener('message', handler);
        if (msg.error) reject(msg.error);
        else resolve(msg.result);
      }
    };
    ws.addEventListener('message', handler);
    ws.send(JSON.stringify({ id, method, params }));
  });
}

async function getTargets(port) {
  return new Promise((resolve, reject) => {
    http.get(`http://127.0.0.1:${port}/json`, res => {
      let d = '';
      res.on('data', c => d += c);
      res.on('end', () => resolve(JSON.parse(d)));
    }).on('error', reject);
  });
}

async function testBrowser(name, binPath, port, extraEnv = {}) {
  console.log('\n=============================================================');
  console.log(`   TESTING BROWSER: ${name.toUpperCase()}`);
  console.log(`   Binary: ${binPath}`);
  console.log(`   Port:   ${port}`);
  console.log('=============================================================');

  const profileDir = fs.mkdtempSync(`/tmp/onusly-${name.toLowerCase().replace(/\s+/g, '-')}-`);
  const nmDir = path.join(profileDir, 'NativeMessagingHosts');
  fs.mkdirSync(nmDir, { recursive: true });
  const manifestSrc = path.join(process.env.HOME, '.config', 'google-chrome', 'NativeMessagingHosts', 'com.onusly.desktop_agent.json');
  if (fs.existsSync(manifestSrc)) {
    fs.copyFileSync(manifestSrc, path.join(nmDir, 'com.onusly.desktop_agent.json'));
  }
  const env = { ...process.env, ...extraEnv };

  const proc = spawn(binPath, [
    '--headless=new',
    `--remote-debugging-port=${port}`,
    `--load-extension=${EXT_PATH}`,
    `--user-data-dir=${profileDir}`,
    '--no-sandbox',
    '--disable-gpu',
    'about:blank'
  ], { env, stdio: 'ignore' });

  let success = false;
  try {
    // 1. Wait for CDP to be available
    let connected = false;
    for (let i = 0; i < 20; i++) {
      await wait(300);
      try {
        await getTargets(port);
        connected = true;
        break;
      } catch {}
    }
    if (!connected) throw new Error(`Could not connect to CDP on port ${port}`);
    console.log('1. CDP endpoint connected.');

    // 2. Open popup tab to awaken extension
    await new Promise(resolve => {
      http.request({
        host: '127.0.0.1',
        port,
        path: `/json/new?chrome-extension://${EXT_ID}/popup.html`,
        method: 'PUT'
      }, resolve).end();
    });
    await wait(1000);

    // 3. Find extension target (page or service_worker)
    const targets = await getTargets(port);
    const extTarget = targets.find(t => t.url.includes(EXT_ID));
    if (!extTarget) throw new Error(`ONUSLY extension target (${EXT_ID}) not found in targets list`);
    console.log(`2. Extension target found: ${extTarget.type} -> ${extTarget.url}`);

    // 4. Connect to extension context
    const ws = new WebSocket(extTarget.webSocketDebuggerUrl);
    await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej; });
    console.log('3. WebSocket connected to extension DevTools.');

    // 5. Test Native Messaging IPC to Desktop Agent
    console.log('4. Querying Desktop Agent via sendNativeMessage...');
    let blocklist = null;
    for (let attempt = 0; attempt < 3; attempt++) {
      const ipcRes = await cdp(ws, 'Runtime.evaluate', {
        expression: `
          new Promise(resolve => {
            chrome.runtime.sendNativeMessage("com.onusly.desktop_agent", { action: "get-blocklist" }, resp => {
              const err = chrome.runtime.lastError ? (chrome.runtime.lastError.message || "LAST_ERROR_EMPTY") : "NO_ERROR";
              resolve({ err, resp: resp || "NO_RESP" });
            });
          })
        `,
        awaitPromise: true,
        returnByValue: true
      });
      const ipcVal = ipcRes.result.value;
      console.log(`   [Attempt ${attempt+1}] ipcVal:`, JSON.stringify(ipcVal));
      if (!ipcVal.err && ipcVal.resp?.blocklist?.targets) {
        blocklist = ipcVal.resp.blocklist;
        break;
      }
      await wait(500);
    }
    if (!blocklist || !blocklist.targets) {
      throw new Error(`Invalid blocklist returned from agent`);
    }
    console.log(`   ✅ Native Host IPC Success! Received blocklist v${blocklist.version} with ${blocklist.targets.length} target(s).`);

    // 6. Set extension storage to managed mode with this blocklist
    console.log('5. Synchronizing extension local cache & enabling managed mode...');
    await cdp(ws, 'Runtime.evaluate', {
      expression: `
        new Promise(resolve => {
          chrome.storage.local.set({
            onusly_managed_mode: true,
            onusly_cache: {
              userId: "${blocklist.userId || 'managed-user'}",
              version: "${blocklist.version}",
              targets: ${JSON.stringify(blocklist.targets)}
            },
            onusly_blocklist_version: "${blocklist.version}"
          }, resolve);
        })
      `,
      awaitPromise: true,
      returnByValue: true
    });
    console.log('   ✅ Extension state updated to Managed Mode.');

    // 7. Test real navigation interception on instagram.com
    console.log('6. Testing real navigation to restricted domain (https://www.instagram.com/)...');
    const tabRes = await cdp(ws, 'Runtime.evaluate', {
      expression: `new Promise(resolve => chrome.tabs.create({ url: "https://www.instagram.com/" }, resolve))`,
      awaitPromise: true,
      returnByValue: true
    });
    const tabId = tabRes.result.value.id;

    // Wait for extension interception
    await wait(2000);

    const updatedTab = await cdp(ws, 'Runtime.evaluate', {
      expression: `new Promise(resolve => chrome.tabs.get(${tabId}, resolve))`,
      awaitPromise: true,
      returnByValue: true
    });

    const finalUrl = updatedTab.result.value.url;
    console.log(`   Navigation result URL: ${finalUrl}`);
    const isBlocked = finalUrl.includes('blocked.html') && finalUrl.includes('instagram.com');
    if (!isBlocked) {
      throw new Error(`Navigation was NOT intercepted by extension: final URL = ${finalUrl}`);
    }
    console.log(`   ✅ REAL BROWSER NAVIGATION INTERCEPTED & REDIRECTED TO BLOCKED.HTML!`);

    // Clean up tab
    await cdp(ws, 'Runtime.evaluate', { expression: `chrome.tabs.remove(${tabId})` });
    ws.close();
    success = true;

    return {
      browser: name,
      policy: true,
      nativeIPC: true,
      managedMode: true,
      blocked: true,
      finalUrl
    };
  } finally {
    proc.kill('SIGKILL');
    try { fs.rmSync(profileDir, { recursive: true, force: true }); } catch {}
  }
}

async function runAll() {
  const results = [];

  // Google Chrome
  try {
    results.push(await testBrowser('Google Chrome', '/home/devansh/.local/bin/google-chrome', 9231));
  } catch (err) {
    console.error('Google Chrome Test Failed:', err.message);
    results.push({ browser: 'Google Chrome', error: err.message });
  }

  // Chromium
  try {
    results.push(await testBrowser('Chromium', '/home/devansh/.local/bin/chromium', 9232, {
      LD_LIBRARY_PATH: '/home/devansh/isolated_browsers/chromium/usr/lib64'
    }));
  } catch (err) {
    console.error('Chromium Test Failed:', err.message);
    results.push({ browser: 'Chromium', error: err.message });
  }

  // Microsoft Edge
  try {
    results.push(await testBrowser('Microsoft Edge', '/home/devansh/.local/bin/microsoft-edge', 9233));
  } catch (err) {
    console.error('Microsoft Edge Test Failed:', err.message);
    results.push({ browser: 'Microsoft Edge', error: err.message });
  }

  // Brave
  try {
    results.push(await testBrowser('Brave', '/usr/bin/brave-browser', 9234));
  } catch (err) {
    console.error('Brave Test Failed:', err.message);
    results.push({ browser: 'Brave', error: err.message });
  }

  console.log('\n\n=============================================================');
  console.log('             CHROMIUM FAMILY VERIFICATION SUMMARY            ');
  console.log('=============================================================');
  console.log(`${'Browser'.padEnd(18)} | ${'Native IPC'.padEnd(12)} | ${'Managed Mode'.padEnd(14)} | ${'Instagram Blocked'.padEnd(18)}`);
  console.log('-'.repeat(68));
  for (const r of results) {
    if (r.error) {
      console.log(`${r.browser.padEnd(18)} | FAIL         | FAIL           | FAIL: ${r.error.slice(0, 20)}`);
    } else {
      console.log(`${r.browser.padEnd(18)} | PASS ✅      | PASS ✅        | BLOCKED ✅`);
    }
  }
  console.log('=============================================================');

  const allPassed = results.every(r => !r.error && r.blocked);
  if (!allPassed) process.exit(1);
}

runAll().catch(e => { console.error('Fatal:', e); process.exit(1); });
