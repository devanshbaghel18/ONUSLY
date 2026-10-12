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

export function setAuth(token, user) {
  if (token) {
    localStorage.setItem("token", token);
    localStorage.setItem("onusly_auth_token", token);
  }
  if (user) {
    const userStr = typeof user === "string" ? user : JSON.stringify(user);
    localStorage.setItem("user", userStr);
    localStorage.setItem("onusly_user", userStr);
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

  // Verify JWT expiration if payload is readable
  try {
    const parts = token.split(".");
    if (parts.length === 3) {
      let base64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
      while (base64.length % 4) {
        base64 += "=";
      }
      const payload = JSON.parse(atob(base64));
      if (payload.exp && Date.now() >= payload.exp * 1000) {
        clearAuth();
        return false;
      }
    }
  } catch {
    // If decoding fails, continue
  }

  return true;
}
