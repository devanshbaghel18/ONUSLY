import { getToken, clearAuth } from "./auth.js";
import { sendExtensionBridgeMessage } from "./extensionBridge.js";

export const API_BASE_URL =
  typeof import.meta !== "undefined" && import.meta.env?.VITE_API_BASE_URL !== undefined
    ? import.meta.env.VITE_API_BASE_URL
    : typeof import.meta !== "undefined" && import.meta.env?.DEV
    ? "http://localhost:8080"
    : "";

export function resolveApiOrigin() {
  if (API_BASE_URL) {
    return API_BASE_URL.replace(/\/+$/, "");
  }
  if (typeof window !== "undefined" && window.location) {
    if (typeof import.meta !== "undefined" && import.meta.env?.DEV) {
      return `${window.location.protocol}//${window.location.hostname}:8080`;
    }
    return window.location.origin;
  }
  return "http://localhost:8080";
}

export function resolveApiUrl(path = "") {
  if (!path) return "";
  if (
    path.startsWith("data:") ||
    path.startsWith("blob:") ||
    path.startsWith("http://") ||
    path.startsWith("https://")
  ) {
    return path;
  }
  const cleanBase = API_BASE_URL.replace(/\/+$/, "");
  const cleanPath = path.startsWith("/") ? path : `/${path}`;
  return `${cleanBase}${cleanPath}`;
}

export function getExtensionApiBaseUrl() {
  const cleanBase = API_BASE_URL.replace(/\/+$/, "");
  if (cleanBase) return cleanBase;
  if (typeof window !== "undefined" && window.location?.origin) {
    return window.location.origin;
  }
  return "http://localhost:8080";
}

async function request(endpoint, options = {}) {
  const token = getToken();
  const headers = {
    "Content-Type": "application/json",
    ...options.headers,
  };

  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    ...options,
    credentials: "include",
    headers,
  });

  if (response.status === 401) {
    clearAuth();
    if (window.location.pathname !== "/") {
      window.location.href = "/";
    }
    throw new Error("Session expired or unauthorized. Please sign in again.");
  }

  if (response.status === 429) {
    const retryAfter = response.headers.get("retry-after") || "60";
    throw new Error(`Rate limit exceeded. Please wait ${retryAfter} seconds before trying again.`);
  }

  if (response.status === 204) {
    return null;
  }

  const contentType = response.headers.get("content-type");
  let data;
  if (contentType && contentType.includes("application/json")) {
    data = await response.json();
  } else {
    data = await response.text();
  }

  if (!response.ok) {
    const errorMsg =
      (typeof data === "object" && (data?.error || data?.message)) ||
      (typeof data === "string" && data) ||
      `Request failed with status ${response.status}`;
    throw new Error(errorMsg);
  }

  return data;
}

export async function loginWithGoogle(credential) {
  return request("/auth/google", {
    method: "POST",
    body: JSON.stringify({
      credential,
      idToken: credential,
    }),
  });
}

export async function logout() {
  try {
    await fetch(`${API_BASE_URL}/auth/logout`, {
      method: "POST",
      credentials: "include",
    });
  } catch {
    // Ignore network failure on logout
  }
  clearAuth();
}

export async function revokeAllSessions() {
  try {
    await request("/auth/revoke-all", {
      method: "POST",
    });
  } catch {
    // Ensure local auth is still cleared even if network/session already invalidated
  }
  clearAuth();
}

export async function getGoals() {
  const data = await request("/goals", { method: "GET" });
  return Array.isArray(data) ? data : [];
}

export async function getGoal(goalId, ownerId = "") {
  const query = ownerId ? `?ownerId=${encodeURIComponent(ownerId)}` : "";
  return request(`/goals/${encodeURIComponent(goalId)}${query}`, {
    method: "GET",
  });
}

export async function createGoal({
  title,
  description = "",
  approvalType,
  approverEmail,
  blockedApps = [],
  blockedDomains = [],
  applicationTargets = [],
}) {
  const payload = {
    title: title.trim(),
    description: description.trim(),
  };
  if (approvalType !== undefined && approvalType !== null) {
    payload.approvalType = approvalType;
  }
  if (approverEmail !== undefined && approverEmail !== null) {
    payload.approverEmail = approverEmail.trim();
  }
  if (blockedApps && blockedApps.length > 0) payload.blockedApps = blockedApps;
  if (blockedDomains && blockedDomains.length > 0) payload.blockedDomains = blockedDomains;
  if (applicationTargets && applicationTargets.length > 0) payload.applicationTargets = applicationTargets;

  return request("/goals", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function deleteGoal(goalId) {
  return request(`/goals/${encodeURIComponent(goalId)}`, {
    method: "DELETE",
  });
}

export async function updateGoal(
  goalId,
  { title, description, approvalType, approverEmail, blockedApps, blockedDomains } = {}
) {
  const payload = {};
  if (title !== undefined) payload.title = title.trim();
  if (description !== undefined) payload.description = description.trim();
  if (approvalType !== undefined) payload.approvalType = approvalType;
  if (approverEmail !== undefined) payload.approverEmail = approverEmail.trim();
  if (blockedApps !== undefined) payload.blockedApps = blockedApps;
  if (blockedDomains !== undefined) payload.blockedDomains = blockedDomains;

  return request(`/goals/${encodeURIComponent(goalId)}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
}

export async function getBlocklist() {
  return request("/enforcement/blocklist", {
    method: "GET",
  });
}

export async function uploadProofFiles(files, goalIdOrOptions = "") {
  if (!files || files.length === 0) return [];
  const opts =
    typeof goalIdOrOptions === "string"
      ? { goalId: goalIdOrOptions }
      : (goalIdOrOptions || {});
  const token = getToken();
  const formData = new FormData();
  if (opts.scope) {
    formData.append("scope", opts.scope);
  }
  if (opts.goalId) {
    formData.append("goalId", opts.goalId);
  }
  if (opts.recipient) {
    formData.append("recipient", opts.recipient);
  }
  if (opts.communityId) {
    formData.append("communityId", opts.communityId);
  }
  for (const f of files) {
    formData.append("files", f);
  }

  const res = await fetch(`${API_BASE_URL}/uploads`, {
    method: "POST",
    credentials: "include",
    headers: {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: formData,
  });

  if (res.status === 401) {
    clearAuth();
    throw new Error("Session expired. Please sign in again.");
  }

  if (!res.ok) {
    const errorText = await res.text();
    throw new Error(errorText || "Failed to upload files");
  }

  return await res.json();
}

export async function submitProof(goalId, { proofType = "text", textExplanation = "", externalLink = "", photoUrl = "", files = null } = {}) {
  return request(`/goals/${encodeURIComponent(goalId)}/proofs`, {
    method: "POST",
    body: JSON.stringify({
      proofType,
      textExplanation,
      externalLink,
      photoUrl,
      files,
    }),
  });
}

export async function getProofs(goalId, ownerId = "") {
  const query = ownerId ? `?ownerId=${encodeURIComponent(ownerId)}` : "";
  const data = await request(`/goals/${encodeURIComponent(goalId)}/proofs${query}`, {
    method: "GET",
  });
  return Array.isArray(data) ? data : [];
}

export async function decideApproval(goalId, proofId, { ownerId, status, comment = "" }) {
  return request(`/goals/${encodeURIComponent(goalId)}/proofs/${encodeURIComponent(proofId)}/approval`, {
    method: "POST",
    body: JSON.stringify({
      ownerId,
      status,
      comment,
    }),
  });
}

export async function getApproverInbox(status = "pending") {
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  const data = await request(`/approvals/inbox${query}`, {
    method: "GET",
  });
  return data || { items: [], total: 0 };
}

export async function getApprovals(goalId) {
  const data = await request(`/goals/${encodeURIComponent(goalId)}/approvals`, {
    method: "GET",
  });
  return Array.isArray(data) ? data : [];
}

export async function getSubmissions(goalId) {
  const data = await request(`/goals/${encodeURIComponent(goalId)}/submissions`, {
    method: "GET",
  });
  return data;
}

export async function getChatHistory(peerEmail) {
  if (!peerEmail) return [];
  const data = await request(`/chat/messages?peer=${encodeURIComponent(peerEmail.trim())}`, {
    method: "GET",
  });
  return Array.isArray(data) ? data : [];
}

export async function sendChatMessageApi(recipient, text, options = {}) {
  const hasMedia =
    Boolean(options.isProof) ||
    (Array.isArray(options.images) && options.images.length > 0) ||
    (Array.isArray(options.files) && options.files.length > 0) ||
    Boolean(options.externalLink);
  if (!recipient || (!text && !hasMedia)) return null;
  return request("/chat/messages", {
    method: "POST",
    body: JSON.stringify({
      recipient,
      text: text || "",
      isProof: options.isProof || false,
      goalId: options.goalId || "",
      goalTitle: options.goalTitle || "",
      ownerId: options.ownerId || "",
      images: Array.isArray(options.images) ? options.images : undefined,
      files: Array.isArray(options.files) ? options.files : undefined,
      externalLink: options.externalLink || undefined,
      replyTo: options.replyTo || undefined,
    }),
  });
}

export async function clearChatHistory(peerEmail) {
  if (!peerEmail) return false;
  return request(`/chat/messages?peer=${encodeURIComponent(peerEmail.trim())}`, {
    method: "DELETE",
  });
}


export async function lookupUser(handle) {
  if (!handle) return null;
  const clean = handle.trim().replace(/^@/, "");
  return request(`/users/search?handle=${encodeURIComponent(clean)}`, {
    method: "GET",
  });
}

export async function updateHandle(newHandle) {
  const clean = (newHandle || "").trim().replace(/^@/, "");
  return request("/users/handle", {
    method: "PATCH",
    body: JSON.stringify({ handle: clean }),
  });
}

export async function getMyProfile() {
  return request("/users/me", {
    method: "GET",
  });
}

export async function getNotifications() {
  const data = await request("/notifications", {
    method: "GET",
  });
  return data || { notifications: [], unreadCount: 0 };
}

export async function markNotificationRead(id) {
  if (!id) return;
  return request(`/notifications/${encodeURIComponent(id)}/read`, {
    method: "PATCH",
  });
}

export async function markAllNotificationsRead() {
  return request("/notifications/read-all", {
    method: "POST",
  });
}

/**
 * Creates a new active cycle of an existing goal (retains title, description,
 * blocked apps, blocked domains, and approver).
 * @param {object} originalGoal
 * @returns {Promise<object>}
 */
export async function repeatGoal(originalGoal) {
  if (!originalGoal || !originalGoal.title) {
    throw new Error("Cannot repeat an invalid goal");
  }

  const newGoal = await createGoal({
    title: originalGoal.title,
    description: originalGoal.description || "",
    approvalType: originalGoal.approvalType || "friend",
    approverEmail: originalGoal.approverEmail || "",
    blockedApps: originalGoal.blockedApps || [],
    blockedDomains: originalGoal.blockedDomains || [],
  });

  return newGoal;
}

export async function sendFriendRequest(target) {
  return request("/friends/request", {
    method: "POST",
    body: JSON.stringify({ target }),
  });
}

export async function getFriendRequests() {
  const data = await request("/friends/requests", { method: "GET" });
  return data || { incoming: [], outgoing: [] };
}

export async function respondFriendRequest(requestId, action) {
  return request("/friends/request/respond", {
    method: "POST",
    body: JSON.stringify({ requestId, action }),
  });
}

export async function getFriendsList() {
  const data = await request("/friends", { method: "GET" });
  return Array.isArray(data) ? data : [];
}

export async function deleteFriendApi(friendId) {
  return request(`/friends/${encodeURIComponent(friendId)}`, {
    method: "DELETE",
  });
}

export async function getTrendingCommunities() {
  const data = await request("/communities/trending", { method: "GET" });
  return Array.isArray(data) ? data : [];
}

export async function getJoinedCommunities() {
  const data = await request("/communities/joined", { method: "GET" });
  return Array.isArray(data) ? data : [];
}

export async function joinCommunityApi(communityId) {
  return request("/communities/join", {
    method: "POST",
    body: JSON.stringify({ communityId }),
  });
}

export async function leaveCommunityApi(communityId) {
  return request("/communities/leave", {
    method: "POST",
    body: JSON.stringify({ communityId }),
  });
}

export async function getDevicesCoverage() {
  try {
    const data = await request("/enforcement/devices", { method: "GET" });
    if (!data) return { userId: "", totalCount: 0, enforcing: false, devices: [] };
    return {
      ...data,
      devices: Array.isArray(data.devices) ? data.devices : [],
    };
  } catch (err) {
    console.warn("[API] getDevicesCoverage failed:", err);
    return { userId: "", totalCount: 0, enforcing: false, devices: [] };
  }
}

export async function getGoalEnforcementStatus(goalId) {
  return request(`/goals/${encodeURIComponent(goalId)}/enforcement-status`, {
    method: "GET",
  });
}

export async function confirmDevicePairing(code) {
  return request("/enforcement/pair/confirm", {
    method: "POST",
    body: JSON.stringify({ code: code.trim().toUpperCase() }),
  });
}

export async function sendHeartbeat(payload) {
  return request("/enforcement/heartbeat", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function revokeDevice(deviceId, reason = "user_revoked") {
  return request(`/enforcement/devices/${encodeURIComponent(deviceId)}/revoke`, {
    method: "POST",
    body: JSON.stringify({ reason }),
  });
}

export function isLoopbackApiOrigin() {
  try {
    const parsed = new URL(resolveApiOrigin());
    const h = (parsed.hostname || "").toLowerCase();
    return h === "localhost" || h === "127.0.0.1" || h === "::1";
  } catch {
    return false;
  }
}

export async function getDesktopAgentStatus() {
  // 1. Prefer local Browser Extension -> Native Messaging Host -> Desktop Agent pipe
  const bridgeRes = await sendExtensionBridgeMessage("ONUSLY_DESKTOP_STATUS", {}, 3500);
  if (bridgeRes && bridgeRes.available) {
    return { available: true, viaExtension: true, status: bridgeRes.status };
  }

  // 2. Only call backend /enforcement/desktop/status when running against localhost dev API
  if (isLoopbackApiOrigin()) {
    try {
      return await request("/enforcement/desktop/status", { method: "GET" });
    } catch (err) {
      return { available: false, error: err.message };
    }
  }

  return {
    available: false,
    remoteApi: true,
    error: bridgeRes?.error || "Desktop agent not reachable via browser extension",
  };
}

export async function toggleDesktopDNS(enabled) {
  if (!isLoopbackApiOrigin()) {
    throw new Error("PermissionDenied: use 1-Click Switcher (.exe) or ONUSLY Desktop CLI to toggle machine-wide DNS.");
  }
  return request("/enforcement/desktop/dns", {
    method: "POST",
    body: JSON.stringify({ enabled }),
  });
}

export async function syncDesktopAgent() {
  const bridgeRes = await sendExtensionBridgeMessage("ONUSLY_DESKTOP_SYNC", {}, 4000);
  if (bridgeRes?.success) {
    return bridgeRes;
  }
  if (isLoopbackApiOrigin()) {
    return request("/enforcement/desktop/sync", { method: "POST" });
  }
  throw new Error(bridgeRes?.error || "Desktop agent could not be reached via browser extension.");
}

export async function autoPairDesktopAgent() {
  const pairReq = await sendExtensionBridgeMessage("ONUSLY_DESKTOP_PAIR_REQUEST", {}, 3000);
  if (pairReq?.success && pairReq.pairCode) {
    await confirmDevicePairing(pairReq.pairCode);
    const pollRes = await sendExtensionBridgeMessage("ONUSLY_DESKTOP_PAIR_POLL", { code: pairReq.pairCode }, 10000);
    if (pollRes?.success) {
      return {
        success: true,
        message: "Desktop agent successfully paired and synchronized!",
        status: pollRes.status,
      };
    }
  }
  if (isLoopbackApiOrigin()) {
    return request("/enforcement/desktop/pair", { method: "POST" });
  }
  throw new Error(pairReq?.error || "Install OnuslySetup.exe or run `onusly-agent pair` and enter the 6-character code.");
}

export async function reconnectDesktopAgent() {
  // 1. Try syncing / checking existing agent via Browser Extension -> Native Host -> IPC
  const syncRes = await sendExtensionBridgeMessage("ONUSLY_DESKTOP_SYNC", {}, 4000);
  if (syncRes?.success) {
    return {
      success: true,
      message: "Desktop agent connected and synchronized via browser extension!",
      status: syncRes.status,
    };
  }

  const statusRes = await sendExtensionBridgeMessage("ONUSLY_DESKTOP_STATUS", {}, 3500);
  if (statusRes?.available && statusRes?.status?.connected) {
    return {
      success: true,
      message: "Desktop agent is active and enforcing!",
      status: statusRes.status,
    };
  }

  // 2. If native host reached an unpaired agent, complete 1-click pairing from the browser session
  const pairReq = await sendExtensionBridgeMessage("ONUSLY_DESKTOP_PAIR_REQUEST", {}, 3000);
  if (pairReq?.success && pairReq.pairCode) {
    await confirmDevicePairing(pairReq.pairCode);
    const pollRes = await sendExtensionBridgeMessage("ONUSLY_DESKTOP_PAIR_POLL", { code: pairReq.pairCode }, 10000);
    if (pollRes?.success) {
      await sendExtensionBridgeMessage("ONUSLY_DESKTOP_SYNC", {}, 3000);
      return {
        success: true,
        message: "Desktop agent successfully paired and synchronized!",
        status: pollRes.status,
      };
    }
    return {
      success: false,
      requiresSetup: true,
      error: pollRes?.error || "Pairing confirmation sent, but desktop agent did not finalize. Retry or use 6-char pairing code.",
    };
  }

  // 3. Local dev server fallback (only on localhost)
  if (isLoopbackApiOrigin()) {
    return request("/enforcement/desktop/reconnect", { method: "POST" });
  }

  return {
    success: false,
    requiresSetup: true,
    error:
      "Could not reach a running local Desktop Agent via browser native messaging. Ensure ONUSLY Desktop is running (or install OnuslySetup.exe), then pair with its 6-character code below.",
  };
}

export async function getWellbeingSummary({ range = "today", deviceId = "" } = {}) {
  const tzOffset = -new Date().getTimezoneOffset();
  const params = new URLSearchParams({
    range,
    tzOffset: String(tzOffset),
  });
  if (deviceId) {
    params.set("deviceId", deviceId);
  }
  return request(`/wellbeing/summary?${params.toString()}`, { method: "GET" });
}

export async function ingestWellbeing(payload) {
  return request("/wellbeing/ingest", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function getWellbeingRules() {
  const tzOffset = -new Date().getTimezoneOffset();
  return request(`/wellbeing/rules?tzOffset=${tzOffset}`, { method: "GET" });
}

export async function upsertWellbeingLimit(limit) {
  return request("/wellbeing/limits", {
    method: "PUT",
    body: JSON.stringify(limit),
  });
}

export async function deleteWellbeingLimit(ruleId) {
  return request(`/wellbeing/limits/${encodeURIComponent(ruleId)}`, {
    method: "DELETE",
  });
}

export async function saveWellbeingSchedule(schedule) {
  const tzOffset = -new Date().getTimezoneOffset();
  return request("/wellbeing/schedule", {
    method: "PUT",
    body: JSON.stringify({
      ...schedule,
      timezoneOffsetMinutes: tzOffset,
    }),
  });
}




