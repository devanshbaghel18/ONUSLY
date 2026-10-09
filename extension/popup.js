/**
 * Logic for ONUSLY extension popup interface.
 */

const statusBadge = document.getElementById("statusBadge");
const statusText = document.getElementById("statusText");

const contentConnected = document.getElementById("contentConnected");
const contentDisconnected = document.getElementById("contentDisconnected");

const userName = document.getElementById("userName");
const userEmail = document.getElementById("userEmail");
const valVersion = document.getElementById("valVersion");
const valLastSync = document.getElementById("valLastSync");
const badgeCount = document.getElementById("badgeCount");
const domainList = document.getElementById("domainList");

const btnSync = document.getElementById("btnSync");
const btnSyncText = document.getElementById("btnSyncText");
const syncSpinner = document.getElementById("syncSpinner");
const btnOpenDashboard = document.getElementById("btnOpenDashboard");
const btnDisconnect = document.getElementById("btnDisconnect");
const btnGoDashboard = document.getElementById("btnGoDashboard");

let dashboardUrl = "http://localhost:5173";

function setSyncing(isSyncing) {
  if (isSyncing) {
    syncSpinner.style.display = "inline-block";
    btnSyncText.textContent = "Syncing...";
    btnSync.disabled = true;
  } else {
    syncSpinner.style.display = "none";
    btnSyncText.textContent = "Sync Now";
    btnSync.disabled = false;
  }
}

async function renderState() {
  try {
    const response = await chrome.runtime.sendMessage({ type: "GET_STATUS" });
    const { session, cache, settings } = response || {};

    if (settings?.dashboardUrl) {
      dashboardUrl = settings.dashboardUrl;
    }

    if (session && session.userId && session.token) {
      statusBadge.className = "status-badge connected";
      statusText.textContent = "Connected";

      contentConnected.style.display = "block";
      contentDisconnected.style.display = "none";

      userName.textContent = session.name || "Authenticated User";
      userEmail.textContent = session.email || "";

      valVersion.textContent = cache?.version || "initial";
      valLastSync.textContent = cache?.lastSyncTimestamp
        ? new Date(cache.lastSyncTimestamp).toLocaleTimeString()
        : "Never";

      // Collect domains
      const targets = cache?.targets || [];
      const domainsMap = new Map();

      for (const t of targets) {
        const status = (t.status || "").toLowerCase().trim();
        if (status !== "active" && status !== "proof_submitted") continue;

        for (const d of t.domains || []) {
          const dom = d.toLowerCase().trim();
          if (!domainsMap.has(dom) || status === "active") {
            domainsMap.set(dom, status);
          }
        }
      }

      badgeCount.textContent = domainsMap.size.toString();
      domainList.innerHTML = "";

      if (domainsMap.size === 0) {
        domainList.innerHTML = '<div class="empty-state">No websites currently blocked.</div>';
      } else {
        const sorted = Array.from(domainsMap.entries()).sort((a, b) => a[0].localeCompare(b[0]));
        for (const [dom, st] of sorted) {
          const item = document.createElement("div");
          item.className = "domain-item";

          const nameSpan = document.createElement("span");
          nameSpan.className = "domain-name";
          nameSpan.textContent = dom;

          const badge = document.createElement("span");
          badge.className = `domain-status ${st === "active" ? "active" : "review"}`;
          badge.textContent = st === "active" ? "Active" : "Review";

          item.appendChild(nameSpan);
          item.appendChild(badge);
          domainList.appendChild(item);
        }
      }
    } else {
      statusBadge.className = "status-badge disconnected";
      statusText.textContent = "Disconnected";

      contentConnected.style.display = "none";
      contentDisconnected.style.display = "block";
    }
  } catch (err) {
    console.error("Failed to load popup state:", err);
  }
}

async function handleSync() {
  setSyncing(true);
  try {
    await chrome.runtime.sendMessage({ type: "SYNC_NOW" });
    await renderState();
  } catch (err) {
    console.error("Sync error:", err);
  } finally {
    setSyncing(false);
  }
}

async function handleDisconnect() {
  try {
    await chrome.runtime.sendMessage({ type: "DISCONNECT" });
    await renderState();
  } catch (err) {
    console.error("Disconnect error:", err);
  }
}

function openDashboard() {
  chrome.tabs.create({ url: dashboardUrl });
}

btnSync.addEventListener("click", handleSync);
btnOpenDashboard.addEventListener("click", openDashboard);
btnGoDashboard.addEventListener("click", openDashboard);
btnDisconnect.addEventListener("click", handleDisconnect);

renderState();
