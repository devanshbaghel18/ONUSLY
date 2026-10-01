export function getToken() {
  return localStorage.getItem("token");
}

export function getUser() {
  const userStr = localStorage.getItem("user");
  if (!userStr) return null;
  try {
    return JSON.parse(userStr);
  } catch (err) {
    console.error("Failed to parse stored user data:", err);
    return null;
  }
}

export function setAuth(token, user) {
  if (token) {
    localStorage.setItem("token", token);
  }
  if (user) {
    localStorage.setItem("user", typeof user === "string" ? user : JSON.stringify(user));
  }
}

export function clearAuth() {
  localStorage.removeItem("token");
  localStorage.removeItem("user");
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
