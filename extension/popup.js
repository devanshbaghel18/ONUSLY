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

      // Also map app names/packages to domains for display
      const APP_MAP = {
        "com.google.android.youtube": ["youtube.com", "youtu.be"],
        "youtube": ["youtube.com", "youtu.be"],
        "com.google.android.youtube.shorts": ["shorts.youtube.com"],
        "youtube_shorts": ["shorts.youtube.com"],
        "youtube-shorts": ["shorts.youtube.com"],
        "com.instagram.android": ["instagram.com"],
        "instagram": ["instagram.com"],
        "com.twitter.android": ["x.com", "twitter.com"],
        "twitter": ["x.com", "twitter.com"],
        "x": ["x.com", "twitter.com"],
        "com.zhiliaoapp.musically": ["tiktok.com"],
        "tiktok": ["tiktok.com"],
        "com.reddit.frontpage": ["reddit.com"],
        "reddit": ["reddit.com"],
        "com.netflix.mediaclient": ["netflix.com"],
        "netflix": ["netflix.com"],
        "com.discord": ["discord.com"],
        "discord": ["discord.com"],
        "tv.twitch.android.app": ["twitch.tv"],
        "twitch": ["twitch.tv"],
      };

      for (const t of targets) {
        const status = (t.status || "").toLowerCase().trim();
        if (status !== "active" && status !== "proof_submitted") continue;

        for (const d of t.domains || []) {
          const dom = d.toLowerCase().trim();
          if (!domainsMap.has(dom) || status === "active") {
            domainsMap.set(dom, status);
          }
        }

        for (const app of t.apps || []) {
          const mapped = APP_MAP[app?.toLowerCase()?.trim()];
          if (mapped) {
            for (const dom of mapped) {
              if (!domainsMap.has(dom) || status === "active") {
                domainsMap.set(dom, status);
              }
            }
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

      // Check if goals are currently in progress to lock Disconnect button
      const hasActiveGoals = targets.some((t) => {
        const st = (t.status || "").toLowerCase().trim();
        return st === "active" || st === "proof_submitted";
      });

      if (btnDisconnect) {
        if (hasActiveGoals) {
          btnDisconnect.disabled = true;
          btnDisconnect.textContent = "🔒 Locked (Goal in Progress)";
          btnDisconnect.title = "Cannot disconnect extension while goals are in progress";
        } else {
          btnDisconnect.disabled = false;
          btnDisconnect.textContent = "Disconnect";
          btnDisconnect.title = "";
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
    const res = await chrome.runtime.sendMessage({ type: "DISCONNECT" });
    if (res && !res.success) {
      alert(res.error || "Cannot disconnect extension while goals are in progress!");
      return;
    }
    await renderState();
  } catch (err) {
    console.error("Disconnect error:", err);
  }
}

function openDashboard() {
  chrome.tabs.create({ url: dashboardUrl });
}

const btnQuickPair = document.getElementById("btnQuickPair");
const btnPairText = document.getElementById("btnPairText");
const pairSpinner = document.getElementById("pairSpinner");
const tokenInput = document.getElementById("tokenInput");
const btnConnectToken = document.getElementById("btnConnectToken");
const tokenError = document.getElementById("tokenError");

function setPairing(isPairing) {
  if (!btnQuickPair) return;
  if (isPairing) {
    if (pairSpinner) pairSpinner.style.display = "inline-block";
    if (btnPairText) btnPairText.textContent = "Connecting...";
    btnQuickPair.disabled = true;
  } else {
    if (pairSpinner) pairSpinner.style.display = "none";
    if (btnPairText) btnPairText.textContent = "⚡ Quick Connect via Tab";
    btnQuickPair.disabled = false;
  }
}

async function handleQuickPair() {
  setPairing(true);
  if (tokenError) tokenError.style.display = "none";

  try {
    // 1. Query for active/open ONUSLY dashboard tabs
    const tabs = await chrome.tabs.query({
      url: [
        "http://localhost:*/*",
        "http://127.0.0.1:*/*",
        "https://*.onusly.com/*"
      ]
    });

    if (!tabs || tabs.length === 0) {
      alert("No ONUSLY dashboard tab found. Please open http://localhost:5173 and log in first.");
      return;
    }

    let token = null;
    let user = null;

    // 2. Try to obtain token from open tab
    for (const tab of tabs) {
      // First try messaging the content script bridge
      try {
        const response = await chrome.tabs.sendMessage(tab.id, { type: "GET_AUTH_FROM_PAGE" });
        if (response?.success && response.token) {
          token = response.token;
          user = response.user;
          break;
        }
      } catch {
        // Fallback to scripting if bridge isn't active
      }

      if (!token && chrome.scripting?.executeScript) {
        try {
          const results = await chrome.scripting.executeScript({
            target: { tabId: tab.id },
            func: () => {
              return {
                token:
                  localStorage.getItem("token") ||
                  localStorage.getItem("onusly_auth_token"),
                user:
                  localStorage.getItem("user") ||
                  localStorage.getItem("onusly_user"),
              };
            },
          });
          if (results?.[0]?.result?.token) {
            token = results[0].result.token;
            try {
              user = JSON.parse(results[0].result.user);
            } catch {
              user = null;
            }
            break;
          }
        } catch {
          // ignore
        }
      }
    }

    if (!token) {
      alert("Could not retrieve login session from the open tab. Please make sure you are logged in to ONUSLY.");
      return;
    }

    // 3. Send handoff to background worker
    const result = await chrome.runtime.sendMessage({
      type: "ONUSLY_AUTH_HANDOFF",
      token,
      user,
      baseUrl: "http://localhost:8080",
    });

    if (result?.success) {
      await renderState();
    } else {
      alert("Pairing failed: " + (result?.error || "Unknown error"));
    }
  } catch (err) {
    console.error("Pairing error:", err);
    alert("Connection error: " + err.message);
  } finally {
    setPairing(false);
  }
}

async function handleConnectToken() {
  const token = (tokenInput?.value || "").trim();
  if (!token) {
    if (tokenError) {
      tokenError.textContent = "Please enter a valid connection token.";
      tokenError.style.display = "block";
    }
    return;
  }

  if (tokenError) tokenError.style.display = "none";

  try {
    const result = await chrome.runtime.sendMessage({
      type: "ONUSLY_AUTH_HANDOFF",
      token,
      baseUrl: "http://localhost:8080",
    });

    if (result?.success) {
      if (tokenInput) tokenInput.value = "";
      await renderState();
    } else {
      if (tokenError) {
        tokenError.textContent = result?.error || "Invalid token.";
        tokenError.style.display = "block";
      }
    }
  } catch (err) {
    if (tokenError) {
      tokenError.textContent = err.message || "Failed to connect.";
      tokenError.style.display = "block";
    }
  }
}

btnSync.addEventListener("click", handleSync);
btnOpenDashboard.addEventListener("click", openDashboard);
btnGoDashboard.addEventListener("click", openDashboard);
btnDisconnect.addEventListener("click", handleDisconnect);
if (btnQuickPair) btnQuickPair.addEventListener("click", handleQuickPair);
if (btnConnectToken) btnConnectToken.addEventListener("click", handleConnectToken);
if (tokenInput) {
  tokenInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") handleConnectToken();
  });
}

renderState();

