/**
 * Logic for the ONUSLY blocked page experience.
 */

import { evaluateUrl } from "./src/blockerEngine.js";
import { isYouTubeShortsUrl } from "./src/domainMatcher.js";

const params = new URLSearchParams(window.location.search);
const targetDomain = params.get("domain") || "Restricted Website";
const originalUrl = params.get("url") || "";

const domainText = document.getElementById("domainText");
const statusCard = document.getElementById("statusCard");
const statusHeader = document.getElementById("statusHeader");
const instructionText = document.getElementById("instructionText");
const goalList = document.getElementById("goalList");
const syncInfo = document.getElementById("syncInfo");
const btnCheckAgain = document.getElementById("btnCheckAgain");
const btnCheckText = document.getElementById("btnCheckText");
const spinner = document.getElementById("spinner");
const btnDashboard = document.getElementById("btnDashboard");
const btnCloseTab = document.getElementById("btnCloseTab");
const btnYouTubeHome = document.getElementById("btnYouTubeHome");

let dashboardUrl = "http://localhost:5173";

function setCheckingState(isChecking) {
  if (isChecking) {
    spinner.style.display = "inline-block";
    btnCheckText.textContent = "Checking...";
    btnCheckAgain.disabled = true;
  } else {
    spinner.style.display = "none";
    btnCheckText.textContent = "Check Again";
    btnCheckAgain.disabled = false;
  }
}

async function renderBlockState() {
  domainText.textContent = targetDomain;

  try {
    const response = await chrome.runtime.sendMessage({ type: "GET_STATUS" });
    const { cache, settings } = response || {};
    if (settings?.dashboardUrl) {
      dashboardUrl = settings.dashboardUrl;
    }

    // Evaluate original URL or domain
    const testUrl = originalUrl || `https://${targetDomain}/`;
    const evaluation = evaluateUrl(testUrl, cache);

    // If no longer blocked (e.g. goal approved or unblocked), navigate back to original URL
    if (!evaluation.isBlocked) {
      let destination = dashboardUrl;
      if (
        originalUrl &&
        !originalUrl.startsWith("chrome-extension://") &&
        !isYouTubeShortsUrl(originalUrl)
      ) {
        destination = originalUrl;
      } else if (
        targetDomain &&
        !targetDomain.includes(" ") &&
        targetDomain !== "Restricted Website"
      ) {
        destination = `https://${targetDomain}/`;
      }
      window.location.replace(destination);
      return;
    }

    // Shorts-specific focus mode experience
    if (evaluation.reason === "youtube_shorts") {
      domainText.textContent = "YouTube Shorts";
      statusCard.className = "status-card shorts-blocked";
      statusHeader.textContent = "STRICT SHORTS MODE";
      instructionText.textContent =
        "YouTube Shorts are blocked so you can stay focused. Regular YouTube videos are still available.";

      goalList.innerHTML = "";
      const li = document.createElement("li");
      const bullet = document.createElement("span");
      bullet.className = "goal-bullet";
      bullet.textContent = "•";
      const titleSpan = document.createElement("span");
      titleSpan.textContent =
        "ONUSLY Strict Shorts Enforcement: direct Shorts navigation is restricted to protect your focus.";
      li.appendChild(bullet);
      li.appendChild(titleSpan);
      goalList.appendChild(li);

      if (btnYouTubeHome) {
        btnYouTubeHome.style.display = "flex";
      }
    } else {
      if (btnYouTubeHome) {
        btnYouTubeHome.style.display = "none";
      }

      // Update goals list safely
      goalList.innerHTML = "";
      if (evaluation.matchingTargets && evaluation.matchingTargets.length > 0) {
        for (const target of evaluation.matchingTargets) {
          const li = document.createElement("li");
          const bullet = document.createElement("span");
          bullet.className = "goal-bullet";
          bullet.textContent = "•";

          const titleSpan = document.createElement("span");
          titleSpan.textContent = target.goalTitle || "Active Goal";

          li.appendChild(bullet);
          li.appendChild(titleSpan);
          goalList.appendChild(li);
        }
      } else {
        const li = document.createElement("li");
        li.textContent = "Active accountability commitment";
        goalList.appendChild(li);
      }

      // Update status card
      if (evaluation.status === "proof_submitted") {
        statusCard.className = "status-card proof-submitted";
        statusHeader.textContent = "PROOF SUBMITTED";
        instructionText.textContent = "Proof submitted. Waiting for review.";
      } else {
        statusCard.className = "status-card";
        statusHeader.textContent = "GOAL IN PROGRESS";
        instructionText.textContent = "Complete your goal and submit proof.";
      }
    }

    // Sync metadata
    if (cache) {
      const syncTime = cache.lastSyncTimestamp
        ? new Date(cache.lastSyncTimestamp).toLocaleTimeString()
        : "Never";
      syncInfo.textContent = `Last synchronized: ${syncTime} • Version: ${cache.version || "initial"}`;
    } else {
      syncInfo.textContent = "Offline cache active";
    }
  } catch (err) {
    console.error("Error loading block status:", err);
    syncInfo.textContent = "Enforcing local blocklist";
  }
}

async function handleCheckAgain() {
  setCheckingState(true);
  try {
    await chrome.runtime.sendMessage({ type: "SYNC_NOW" });
    await renderBlockState();
  } catch (err) {
    console.error("Sync failed:", err);
  } finally {
    setCheckingState(false);
  }
}

btnCheckAgain.addEventListener("click", handleCheckAgain);

btnDashboard.addEventListener("click", () => {
  window.location.href = dashboardUrl;
});

if (btnYouTubeHome) {
  btnYouTubeHome.addEventListener("click", () => {
    window.location.replace("https://www.youtube.com/");
  });
}

btnCloseTab.addEventListener("click", async () => {
  try {
    const currentTab = await chrome.tabs.getCurrent();
    if (currentTab?.id) {
      chrome.tabs.remove(currentTab.id);
    } else {
      window.close();
    }
  } catch {
    window.close();
  }
});

// Initial render and immediate verification sync
async function init() {
  await renderBlockState();
  // Immediately verify with server in case goal status changed recently
  try {
    await chrome.runtime.sendMessage({ type: "SYNC_NOW" });
    await renderBlockState();
  } catch (err) {
    console.warn("[ONUSLY] Initial verification sync skipped:", err);
  }
}

init();

// Auto-check ticker every 5 seconds: actively checks with backend for approved goals
setInterval(async () => {
  try {
    await chrome.runtime.sendMessage({ type: "SYNC_NOW" });
    await renderBlockState();
  } catch {
    // Ignore background auto-sync network errors
  }
}, 5000);
