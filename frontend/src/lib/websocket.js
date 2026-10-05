import { getToken, getUser, isAuthenticated } from "./auth";

function getWebSocketBaseUrl() {
  if (import.meta.env.VITE_WS_BASE_URL) {
    return import.meta.env.VITE_WS_BASE_URL;
  }

  const apiBase =
    import.meta.env.VITE_API_BASE_URL !== undefined
      ? import.meta.env.VITE_API_BASE_URL
      : import.meta.env.DEV
      ? "http://localhost:8080"
      : "";

  if (apiBase.startsWith("https://")) {
    return apiBase.replace("https://", "wss://");
  }
  if (apiBase.startsWith("http://")) {
    return apiBase.replace("http://", "ws://");
  }

  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}`;
}

class WebSocketManager {
  constructor() {
    this.socket = null;
    this.subscribers = new Set();
    this.statusListeners = new Set();
    this.reconnectTimeout = null;
    this.reconnectAttempts = 0;
    this.maxReconnectDelay = 10000;
    this.isManuallyClosed = false;
    this.isConnected = false;
  }

  connect() {
    if (this.socket && (this.socket.readyState === WebSocket.OPEN || this.socket.readyState === WebSocket.CONNECTING)) {
      return;
    }

    const token = getToken();
    if (!token || !isAuthenticated()) {
      return;
    }

    this.isManuallyClosed = false;
    const baseUrl = getWebSocketBaseUrl().replace(/\/+$/, "");
    const wsUrl = `${baseUrl}/ws?token=${encodeURIComponent(token)}`;

    try {
      this.socket = new WebSocket(wsUrl);

      this.socket.onopen = () => {
        this.isConnected = true;
        this.reconnectAttempts = 0;
        this._notifyStatus(true);
        console.log("[WebSocket] Connected to realtime gateway");

        const user = getUser();
        if (user?.email) {
          this.send({
            type: "user.online",
            payload: { email: user.email },
          });
        }
      };

      this.socket.onmessage = (event) => {
        try {
          const parsed = JSON.parse(event.data);
          this._notifySubscribers(parsed);
        } catch (err) {
          console.error("[WebSocket] Failed to parse message:", err, event.data);
        }
      };

      this.socket.onclose = () => {
        this.isConnected = false;
        this._notifyStatus(false);
        this.socket = null;

        if (!this.isManuallyClosed && isAuthenticated()) {
          this._scheduleReconnect();
        }
      };

      this.socket.onerror = (err) => {
        console.warn("[WebSocket] Socket error:", err);
      };
    } catch (err) {
      console.error("[WebSocket] Connection attempt failed:", err);
      this._scheduleReconnect();
    }
  }

  disconnect() {
    this.isManuallyClosed = true;
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
    if (this.socket) {
      this.socket.close();
      this.socket = null;
    }
    this.isConnected = false;
    this._notifyStatus(false);
  }

  subscribe(callback) {
    this.subscribers.add(callback);
    // Connect on first subscriber if not connected
    if (!this.isConnected && isAuthenticated()) {
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
    if (this.socket && this.socket.readyState === WebSocket.OPEN) {
      this.socket.send(typeof data === "string" ? data : JSON.stringify(data));
      return true;
    }
    return false;
  }

  _scheduleReconnect() {
    if (this.reconnectTimeout || this.isManuallyClosed) return;

    this.reconnectAttempts++;
    const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts - 1), this.maxReconnectDelay);
    console.log(`[WebSocket] Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})...`);

    this.reconnectTimeout = setTimeout(() => {
      this.reconnectTimeout = null;
      this.connect();
    }, delay);
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
