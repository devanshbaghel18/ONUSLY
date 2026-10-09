/**
 * HTTP client for ONUSLY enforcement blocklist endpoint.
 */

/**
 * Fetches the user's active blocklist from the backend API.
 * Supports ETag / If-None-Match caching.
 *
 * @param {string} baseUrl - Backend API base URL.
 * @param {string} token - ONUSLY JWT token.
 * @param {string|null} [ifNoneMatch] - Cached version string to check with If-None-Match.
 * @param {Function} [fetchFn] - Optional fetch function for testing.
 * @returns {Promise<{
 *   type: "SUCCESS"|"NOT_MODIFIED"|"ERROR",
 *   data?: object,
 *   version?: string,
 *   status?: number,
 *   message?: string
 * }>}
 */
export async function fetchBlocklist(
  baseUrl,
  token,
  ifNoneMatch = null,
  fetchFn = globalThis.fetch
) {
  if (!baseUrl || !token) {
    return {
      type: "ERROR",
      status: 400,
      message: "Missing baseUrl or token",
    };
  }

  const cleanBase = baseUrl.trim().replace(/\/+$/, "");
  const url = `${cleanBase}/enforcement/blocklist`;

  const headers = {
    Authorization: `Bearer ${token}`,
    Accept: "application/json",
  };

  if (ifNoneMatch && typeof ifNoneMatch === "string" && ifNoneMatch.trim()) {
    headers["If-None-Match"] = ifNoneMatch.trim();
  }

  try {
    const response = await fetchFn(url, {
      method: "GET",
      headers,
    });

    if (response.status === 304) {
      return { type: "NOT_MODIFIED" };
    }

    if (response.status === 200) {
      const data = await response.json();
      const etagHeader = response.headers?.get?.("ETag");
      const cleanEtag = etagHeader ? etagHeader.replace(/^[Ww]\//, "").replace(/"/g, "").trim() : null;
      const version = data.version || cleanEtag || "";

      return {
        type: "SUCCESS",
        data: {
          ...data,
          version,
        },
        version,
      };
    }

    let errorMessage = `HTTP ${response.status}`;
    try {
      const errBody = await response.json();
      if (errBody?.error || errBody?.message) {
        errorMessage = errBody.error || errBody.message;
      }
    } catch {
      try {
        const text = await response.text();
        if (text) errorMessage = text;
      } catch {
        // Fallback to HTTP status
      }
    }

    return {
      type: "ERROR",
      status: response.status,
      message: errorMessage,
    };
  } catch (err) {
    if (ifNoneMatch) {
      try {
        const fallbackHeaders = {
          Authorization: `Bearer ${token}`,
          Accept: "application/json",
        };
        const fallbackRes = await fetchFn(url, {
          method: "GET",
          headers: fallbackHeaders,
        });

        if (fallbackRes.status === 200) {
          const data = await fallbackRes.json();
          const etagHeader = fallbackRes.headers?.get?.("ETag");
          const cleanEtag = etagHeader ? etagHeader.replace(/^[Ww]\//, "").replace(/"/g, "").trim() : null;
          const version = data.version || cleanEtag || "";

          return {
            type: "SUCCESS",
            data: {
              ...data,
              version,
            },
            version,
          };
        }
      } catch {
        // Fallback also failed, proceed with original error
      }
    }

    return {
      type: "ERROR",
      status: -1,
      message: err.message || "Network request failed",
    };
  }
}
