import { getToken, clearAuth } from "./auth";

const API_BASE_URL =
  import.meta.env.VITE_API_BASE_URL !== undefined
    ? import.meta.env.VITE_API_BASE_URL
    : import.meta.env.DEV
    ? "http://localhost:8080"
    : "";

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
    headers,
  });

  if (response.status === 401) {
    clearAuth();
    if (window.location.pathname !== "/") {
      window.location.href = "/";
    }
    throw new Error("Session expired or unauthorized. Please sign in again.");
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

export async function getGoals() {
  const data = await request("/goals", { method: "GET" });
  return Array.isArray(data) ? data : [];
}

export async function getGoal(goalId) {
  return request(`/goals/${encodeURIComponent(goalId)}`, {
    method: "GET",
  });
}

export async function createGoal({ title, description = "" }) {
  return request("/goals", {
    method: "POST",
    body: JSON.stringify({
      title: title.trim(),
      description: description.trim(),
    }),
  });
}

export async function deleteGoal(goalId) {
  return request(`/goals/${encodeURIComponent(goalId)}`, {
    method: "DELETE",
  });
}

export async function updateGoal(
  goalId,
  { title, description, approvalType, approverEmail } = {}
) {
  const payload = {};
  if (title !== undefined) payload.title = title.trim();
  if (description !== undefined) payload.description = description.trim();
  if (approvalType !== undefined) payload.approvalType = approvalType;
  if (approverEmail !== undefined) payload.approverEmail = approverEmail.trim();

  return request(`/goals/${encodeURIComponent(goalId)}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
}
