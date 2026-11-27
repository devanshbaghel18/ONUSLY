import { notifyExtensionSync } from "./extensionSync.js";

export function getToken() {
  return localStorage.getItem("token");
}

export function getUser() {
  let user = null;
  const userStr = localStorage.getItem("user");
  if (userStr && userStr !== "null" && userStr !== "undefined") {
    try {
      user = JSON.parse(userStr);
    } catch (err) {
      console.error("Failed to parse stored user data:", err);
    }
  }

  // Fallback: decode JWT token payload for id and email
  const token = getToken();
  if (token) {
    try {
      const parts = token.split(".");
      if (parts.length === 3) {
        let base64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
        while (base64.length % 4) {
          base64 += "=";
        }
        const payload = JSON.parse(atob(base64));
        if (!user) {
          user = {
            id: payload.sub,
            email: payload.email,
            handle: payload.handle || (payload.email ? payload.email.split("@")[0] : ""),
            name: payload.name || (payload.email ? payload.email.split("@")[0] : "User"),
          };
        } else {
          if (!user.email && payload.email) user.email = payload.email;
          if (!user.id && payload.sub) user.id = payload.sub;
          if (!user.handle && payload.handle) user.handle = payload.handle;
        }
      }
    } catch {
      // Ignore token decode errors
    }
  }

  return user;
}

export function updateUserHandle(newHandle) {
  const clean = (newHandle || "").trim().replace(/^@/, "");
  let user = getUser() || {};
  user.handle = clean;
  localStorage.setItem("user", JSON.stringify(user));
  return user;
}

function clearUserScopedChatCache() {
  try {
    localStorage.removeItem("onusly_friends_list");
    localStorage.removeItem("onusly_friends_chats");
    localStorage.removeItem("onusly_friends_chats_cleared_at");
    localStorage.removeItem("onusly_communities");
    localStorage.removeItem("onusly_comm_chats");
    localStorage.removeItem("onusly_comm_chats_cleared_at");
  } catch {
    // Ignore storage removal errors
  }
}

export function setAuth(token, user) {
  const prevUid = localStorage.getItem("onusly_last_auth_uid") || "";
  const parsedUser = typeof user === "string" ? (() => { try { return JSON.parse(user); } catch { return null; } })() : user;
  const nextUid = String(parsedUser?.id || parsedUser?.sub || parsedUser?.email || "").trim().toLowerCase();

  // Prevent local friend/chat cache from leaking across different accounts in the same browser
  if (prevUid && nextUid && prevUid !== nextUid) {
    clearUserScopedChatCache();
  }
  if (nextUid) {
    localStorage.setItem("onusly_last_auth_uid", nextUid);
  }

  if (token) {
    localStorage.setItem("token", token);
    localStorage.setItem("onusly_auth_token", token);
  }
  if (user) {
    const userStr = typeof user === "string" ? user : JSON.stringify(user);
    localStorage.setItem("user", userStr);
    localStorage.setItem("onusly_user", userStr);
  }
  // Immediately sync extension when user authenticates
  try {
    notifyExtensionSync();
  } catch {
    // Ignore extension sync error in non-extension contexts
  }
}

let authCleanupHandlers = new Set();

export function registerAuthCleanup(handler) {
  if (typeof handler === "function") {
    authCleanupHandlers.add(handler);
  }
}

export function clearAuth() {
  localStorage.removeItem("token");
  localStorage.removeItem("onusly_auth_token");
  localStorage.removeItem("user");
  localStorage.removeItem("onusly_user");
  localStorage.removeItem("onusly_last_auth_uid");
  clearUserScopedChatCache();

  // Asynchronously instruct backend to clear HttpOnly session cookie
  try {
    const baseUrl =
      typeof import.meta !== "undefined" && import.meta.env?.VITE_API_BASE_URL !== undefined
        ? import.meta.env.VITE_API_BASE_URL.replace(/\/+$/, "")
        : typeof import.meta !== "undefined" && import.meta.env?.DEV
        ? "http://localhost:8080"
        : typeof window !== "undefined" && window.location
        ? window.location.origin
        : "";
    fetch(`${baseUrl}/auth/logout`, {
      method: "POST",
      credentials: "include",
    }).catch(() => {});
  } catch {
    // Ignore logout beacon errors
  }

  authCleanupHandlers.forEach((handler) => {
    try {
      handler();
    } catch (err) {
      console.warn("[Auth] Cleanup handler failed:", err);
    }
  });
}

export function isAuthenticated() {
  const token = getToken();
  if (!token) return false;

  // Verify JWT structure and expiration strictly; fail closed on malformed tokens
  try {
    const parts = token.split(".");
    if (parts.length !== 3 || !parts[1]) {
      clearAuth();
      return false;
    }
    let base64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    while (base64.length % 4) {
      base64 += "=";
    }
    const payload = JSON.parse(atob(base64));
    if (!payload || typeof payload !== "object") {
      clearAuth();
      return false;
    }
    if (payload.exp && Date.now() >= payload.exp * 1000) {
      clearAuth();
      return false;
    }
    return true;
  } catch {
    clearAuth();
    return false;
  }
}
