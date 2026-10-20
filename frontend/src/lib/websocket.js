import { getToken, getUser, isAuthenticated, registerAuthCleanup } from "./auth.js";

export const ConnectionState = Object.freeze({
  DISCONNECTED: "DISCONNECTED",
  CONNECTING: "CONNECTING",
  AUTHENTICATING: "AUTHENTICATING",
  AUTHENTICATED: "AUTHENTICATED",
});

const MAX_QUEUE_SIZE = 100;
const AUTH_TIMEOUT_MS = 6000;

function getWebSocketBaseUrl() {
  if (typeof import.meta !== "undefined" && import.meta.env?.VITE_WS_BASE_URL) {
    return import.meta.env.VITE_WS_BASE_URL;
  }

  const apiBase =
    typeof import.meta !== "undefined" && import.meta.env?.VITE_API_BASE_URL !== undefined
      ? import.meta.env.VITE_API_BASE_URL
      : typeof import.meta !== "undefined" && import.meta.env?.DEV
      ? "http://localhost:8080"
      : "";

  if (apiBase.startsWith("https://")) {
    return apiBase.replace("https://", "wss://");
  }
  if (apiBase.startsWith("http://")) {
    return apiBase.replace("http://", "ws://");
  }

  if (typeof window !== "undefined" && window.location) {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    return `${protocol}//${window.location.host}`;
  }

  return "ws://localhost:8080";
}

export class WebSocketManager {
  constructor(options = {}) {
    this.createSocket = options.createSocket || ((url) => new WebSocket(url));
    this.getWsBaseUrl = options.getWsBaseUrl || getWebSocketBaseUrl;
    this.getToken = options.getToken || getToken;
    this.getUser = options.getUser || getUser;
    this.isAuthenticated = options.isAuthenticated || isAuthenticated;

    this.socket = null;
    this.state = ConnectionState.DISCONNECTED;
    this.connectionGeneration = 0;
    this.authTimeoutTimer = null;
    this.authenticatedUserId = null;

    this.subscribers = new Set();
    this.statusListeners = new Set();
    this.reconnectTimeout = null;
    this.reconnectAttempts = 0;
    this.maxReconnectDelay = 3000;
    this.isManuallyClosed = false;
    this.pendingQueue = [];
  }

  get isConnected() {
    return this.state === ConnectionState.AUTHENTICATED;
  }

  connect() {
    // If already in-flight or connected, do nothing
    const isSocketConnectingOrOpen =
      this.socket &&
      (this.socket.readyState === (typeof WebSocket !== "undefined" ? WebSocket.OPEN : 1) ||
        this.socket.readyState === (typeof WebSocket !== "undefined" ? WebSocket.CONNECTING : 0));

    if (
      isSocketConnectingOrOpen &&
      (this.state === ConnectionState.CONNECTING ||
        this.state === ConnectionState.AUTHENTICATING ||
        this.state === ConnectionState.AUTHENTICATED)
    ) {
      return;
    }

    const token = this.getToken();
    if (!token || !this.isAuthenticated()) {
      this.disconnect();
      return;
    }

    // Guard against account mismatch: if currentUser changed from previous authenticatedUserId
    const currentUser = this.getUser();
    const currentUserId = currentUser?.id || currentUser?.sub || null;
    if (this.authenticatedUserId && currentUserId && this.authenticatedUserId !== currentUserId) {
      this.pendingQueue = [];
    }

    this.isManuallyClosed = false;
    this._clearAuthTimeout();

    const generation = ++this.connectionGeneration;
    this.state = ConnectionState.CONNECTING;
    this._notifyStatus(false);

    const baseUrl = this.getWsBaseUrl().replace(/\/+$/, "");
    const wsUrl = `${baseUrl}/ws`;

    try {
      const ws = this.createSocket(wsUrl);
      this.socket = ws;

      // Start client-side authentication timeout timer
      this.authTimeoutTimer = setTimeout(() => {
        if (generation === this.connectionGeneration && this.state !== ConnectionState.AUTHENTICATED) {
          console.warn("[WebSocket] Authentication timed out on client");
          this._handleAuthFailure(ws, generation, "Authentication timed out");
        }
      }, AUTH_TIMEOUT_MS);
      if (typeof this.authTimeoutTimer?.unref === "function") {
        this.authTimeoutTimer.unref();
      }

      ws.onopen = () => {
        if (generation !== this.connectionGeneration) {
          try {
            ws.close();
          } catch (err) {
            console.debug("[WebSocket] Close on superseded generation ignored:", err);
          }
          return;
        }

        // Transition to AUTHENTICATING state
        this.state = ConnectionState.AUTHENTICATING;

        // CRITICAL: Send authentication frame first.
        // Never send application messages or flush pendingQueue before auth.success.
        try {
          ws.send(
            JSON.stringify({
              type: "auth",
              token: token,
            })
          );
        } catch (err) {
          console.error("[WebSocket] Failed sending auth frame:", err);
          this._handleAuthFailure(ws, generation, "Failed sending auth frame");
        }
      };

      ws.onmessage = (event) => {
        if (generation !== this.connectionGeneration) {
          return;
        }

        let parsed;
        try {
          parsed = typeof event.data === "string" ? JSON.parse(event.data) : event.data;
        } catch (err) {
          console.error("[WebSocket] Failed to parse message:", err, event.data);
          return;
        }

        // Handle server handshake responses
        if (parsed?.type === "auth.success") {
          this._clearAuthTimeout();

          // Exactly-once guard
          if (this.state === ConnectionState.AUTHENTICATED) {
            return;
          }

          this.state = ConnectionState.AUTHENTICATED;
          this.reconnectAttempts = 0;
          this.authenticatedUserId = parsed?.payload?.userId || currentUserId;
          this._notifyStatus(true);
          console.log("[WebSocket] Authenticated with realtime gateway");

          // Announce presence after authentication is confirmed
          const user = this.getUser();
          if (user) {
            const cleanH = user.handle ? user.handle.replace(/[@\s]/g, "").toLowerCase() : "";
            this._sendRaw({
              type: "user.online",
              payload: {
                email: user.email || "",
                handle: cleanH,
              },
            });
          }

          // Flush permitted queued operations in order
          const toSend = [...this.pendingQueue];
          this.pendingQueue = [];
          for (const item of toSend) {
            this._sendRaw(item);
          }
          return;
        }

        if (
          parsed?.type === "auth.error" ||
          (parsed?.type === "error" && parsed?.payload?.code === "UNAUTHORIZED")
        ) {
          const reason = parsed?.payload?.message || "Authentication rejected";
          this._handleAuthFailure(ws, generation, reason);
          return;
        }

        // Only dispatch application messages when in AUTHENTICATED state
        if (this.state === ConnectionState.AUTHENTICATED) {
          this._notifySubscribers(parsed);
        }
      };

      ws.onclose = () => {
        if (generation !== this.connectionGeneration) {
          return;
        }

        this._clearAuthTimeout();
        const wasAuthenticated = this.state === ConnectionState.AUTHENTICATED;
        this.state = ConnectionState.DISCONNECTED;
        this.socket = null;
        this._notifyStatus(false);

        // If connection dropped before reaching AUTHENTICATED, discard unsafe pending operations
        if (!wasAuthenticated) {
          this.pendingQueue = [];
        }

        if (!this.isManuallyClosed && this.isAuthenticated()) {
          this._scheduleReconnect();
        }
      };

      ws.onerror = (err) => {
        if (generation !== this.connectionGeneration) {
          return;
        }
        console.warn("[WebSocket] Socket error:", err);
      };
    } catch (err) {
      console.error("[WebSocket] Connection attempt failed:", err);
      this._clearAuthTimeout();
      this.state = ConnectionState.DISCONNECTED;
      this.socket = null;
      this.pendingQueue = [];
      this._notifyStatus(false);
      this._scheduleReconnect();
    }
  }

  _clearAuthTimeout() {
    if (this.authTimeoutTimer) {
      clearTimeout(this.authTimeoutTimer);
      this.authTimeoutTimer = null;
    }
  }

  _handleAuthFailure(ws, generation, reason) {
    if (generation !== this.connectionGeneration) return;

    this._clearAuthTimeout();
    this.isManuallyClosed = true;
    this.state = ConnectionState.DISCONNECTED;
    this.authenticatedUserId = null;
    // Discard pending operations on auth failure
    this.pendingQueue = [];
    this._notifyStatus(false);

    try {
      ws.close(4403, reason);
    } catch (err) {
      console.debug("[WebSocket] Close during auth failure ignored:", err);
    }
    this.socket = null;
  }

  disconnect() {
    this.isManuallyClosed = true;
    this.connectionGeneration++;
    this._clearAuthTimeout();

    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }

    if (this.socket) {
      try {
        this.socket.close();
      } catch (err) {
        console.debug("[WebSocket] Close during disconnect ignored:", err);
      }
      this.socket = null;
    }

    this.state = ConnectionState.DISCONNECTED;
    this.authenticatedUserId = null;
    this.pendingQueue = [];
    this._notifyStatus(false);
  }

  subscribe(callback) {
    this.subscribers.add(callback);
    // Connect on first subscriber if disconnected and authenticated
    if (this.state === ConnectionState.DISCONNECTED && this.isAuthenticated()) {
      this.connect();
    }

    return () => {
      this.subscribers.delete(callback);
    };
  }

  onStatusChange(callback) {
    this.statusListeners.add(callback);
    callback(this.isConnected);
    return () => {
      this.statusListeners.delete(callback);
    };
  }

  send(data) {
    const serialized = typeof data === "string" ? data : JSON.stringify(data);

    // If fully authenticated and socket is ready, send immediately
    const isOpen =
      this.socket &&
      this.socket.readyState === (typeof WebSocket !== "undefined" ? WebSocket.OPEN : 1);

    if (this.state === ConnectionState.AUTHENTICATED && isOpen) {
      return this._sendRaw(serialized);
    }

    // Not authenticated yet: check user auth validity
    if (!this.isAuthenticated()) {
      this.pendingQueue = [];
      return false;
    }

    // Queue bounded to MAX_QUEUE_SIZE
    if (this.pendingQueue.length >= MAX_QUEUE_SIZE) {
      this.pendingQueue.shift(); // Discard oldest
    }
    this.pendingQueue.push(serialized);

    // Trigger connection if currently disconnected
    if (this.state === ConnectionState.DISCONNECTED) {
      this.connect();
    }

    return false;
  }

  _sendRaw(data) {
    try {
      const isOpen =
        this.socket &&
        this.socket.readyState === (typeof WebSocket !== "undefined" ? WebSocket.OPEN : 1);
      if (isOpen) {
        const payload = typeof data === "string" ? data : JSON.stringify(data);
        this.socket.send(payload);
        return true;
      }
    } catch (err) {
      console.warn("[WebSocket] Error sending message:", err);
    }
    return false;
  }

  _scheduleReconnect() {
    if (this.reconnectTimeout || this.isManuallyClosed) return;

    this.reconnectAttempts++;
    const delay = Math.min(400 * Math.pow(1.6, this.reconnectAttempts - 1), this.maxReconnectDelay);
    console.log(`[WebSocket] Reconnecting in ${Math.round(delay)}ms (attempt ${this.reconnectAttempts})...`);

    this.reconnectTimeout = setTimeout(() => {
      this.reconnectTimeout = null;
      this.connect();
    }, delay);
    if (typeof this.reconnectTimeout?.unref === "function") {
      this.reconnectTimeout.unref();
    }
  }

  _notifySubscribers(event) {
    for (const callback of this.subscribers) {
      try {
        callback(event);
      } catch (err) {
        console.error("[WebSocket] Subscriber handler threw error:", err);
      }
    }
  }

  _notifyStatus(status) {
    for (const listener of this.statusListeners) {
      try {
        listener(status);
      } catch (err) {
        console.error("[WebSocket] Status listener error:", err);
      }
    }
  }
}

export const wsManager = new WebSocketManager();

// Automatically disconnect WebSocket and clear queues when user logs out or session expires
registerAuthCleanup(() => {
  wsManager.disconnect();
});
