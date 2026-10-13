import { useState, useEffect, useCallback } from "react";
import {
  Shield,
  AlertCircle,
  RefreshCw,
  Unlink,
  CheckCircle2,
  Copy,
  Check,
  Download,
  X,
  FolderArchive,
} from "lucide-react";
import { getToken, getUser } from "../lib/auth";
import { getBlocklist, API_BASE_URL } from "../lib/api";
import { notifyExtensionSync } from "../lib/extensionSync";

// Deterministic Extension ID from manifest.json public key
export const DEFAULT_EXTENSION_ID = "gkichkblphmdbojdjnonljlegnmjcnhg";

function getStorageKey(userId) {
  return `onusly_ext_conn_${userId || "default"}`;
}

function getStoredConnection(userId) {
  try {
    const raw = localStorage.getItem(getStorageKey(userId));
    if (!raw) return null;
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

function saveStoredConnection(userId, data) {
  try {
    localStorage.setItem(getStorageKey(userId), JSON.stringify(data));
  } catch {
    // Fail silently
  }
}

function clearStoredConnection(userId) {
  try {
    localStorage.removeItem(getStorageKey(userId));
  } catch {
    // Fail silently
  }
}

export default function ConnectExtension() {
  const user = getUser();
  const userId = user?.id || "";

  const [extensionId, setExtensionId] = useState(() => {
    return localStorage.getItem("onusly_extension_id") || DEFAULT_EXTENSION_ID;
  });
  const [showConfig, setShowConfig] = useState(false);
  const [copiedToken, setCopiedToken] = useState(false);
  const [showInstallModal, setShowInstallModal] = useState(false);

  // Status: "connected" | "disconnected"
  const [status, setStatus] = useState(() => {
    const stored = getStoredConnection(userId);
    return stored?.connected ? "connected" : "disconnected";
  });

  const [extInfo, setExtInfo] = useState(() => {
    return getStoredConnection(userId);
  });
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");
  const [messageType, setMessageType] = useState("info"); // "info" | "success" | "error"

  // Check initial connection and listen for content script bridge announcements
  useEffect(() => {
    let isMounted = true;
    let verified = false;

    // 1. Listen for extension announcements and ping responses from bridge
    const handleBridgeMessage = (event) => {
      if (event.source !== window || !event.data || typeof event.data !== "object") return;

      if (event.data.type === "ONUSLY_EXTENSION_ANNOUNCE" && event.data.extensionId) {
        if (event.data.extensionId !== extensionId) {
          setExtensionId(event.data.extensionId);
          localStorage.setItem("onusly_extension_id", event.data.extensionId);
        }
      }

      if (event.data.type === "ONUSLY_PING_RESPONSE") {
        const resp = event.data.response;
        if (resp?.status === "ok") {
          verified = true;
          if (resp.connected && resp.userId === userId) {
            if (isMounted) {
              setStatus("connected");
              const freshInfo = {
                connected: true,
                version: resp.version || "active",
                targetsCount: resp.targetsCount ?? 0,
                connectedAt: Date.now(),
              };
              setExtInfo(freshInfo);
              saveStoredConnection(userId, freshInfo);
            }
          } else {
            // Extension is active but not connected or belongs to another user
            if (isMounted) {
              setStatus("disconnected");
              setExtInfo(null);
              clearStoredConnection(userId);
            }
          }
        }
      }
    };
    window.addEventListener("message", handleBridgeMessage);

    // 2. Ping real extension to update live status if available
    const checkLiveExtension = () => {
      // Direct chrome.runtime ping
      if (window.chrome?.runtime?.sendMessage) {
        try {
          window.chrome.runtime.sendMessage(
            extensionId,
            { type: "ONUSLY_PING" },
            (response) => {
              if (!isMounted) return;
              if (window.chrome.runtime.lastError) return;

              if (response?.status === "ok") {
                verified = true;
                if (response.connected && response.userId === userId) {
                  setStatus("connected");
                  const freshInfo = {
                    connected: true,
                    version: response.version || "active",
                    targetsCount: response.targetsCount ?? 0,
                    connectedAt: Date.now(),
                  };
                  setExtInfo(freshInfo);
                  saveStoredConnection(userId, freshInfo);
                } else {
                  setStatus("disconnected");
                  setExtInfo(null);
                  clearStoredConnection(userId);
                }
              }
            }
          );
        } catch {
          // Suppress runtime error
        }
      }

      // Content script bridge ping
      window.postMessage({ type: "ONUSLY_PING" }, "*");

      // 3. Fallback verification: if after 2000ms the extension has not answered,
      // invalidate any false stored connection state.
      setTimeout(() => {
        if (isMounted && !verified) {
          setStatus("disconnected");
          setExtInfo(null);
          clearStoredConnection(userId);
        }
      }, 2000);
    };

    checkLiveExtension();

    return () => {
      isMounted = false;
      window.removeEventListener("message", handleBridgeMessage);
    };
  }, [extensionId, userId]);

  // Connect Extension Action
  const handleConnect = useCallback(async () => {
    const token = getToken();
    const currentUser = getUser();

    if (!token) {
      setMessage("Please sign in to ONUSLY first.");
      setMessageType("error");
      return;
    }

    setLoading(true);
    setMessage("");

    // Step 1: Fetch live blocklist targets count from backend API
    let targetsCount = 0;
    let blocklistVersion = "active";
    try {
      const blocklist = await getBlocklist();
      targetsCount = Array.isArray(blocklist?.targets) ? blocklist.targets.length : 0;
      if (blocklist?.version) blocklistVersion = blocklist.version;
    } catch {
      // Fallback
    }

    // Step 2: Handoff credentials to the extension service worker
    // Helper to send handoff via chrome.runtime
    const sendViaRuntime = () => {
      return new Promise((resolve) => {
        if (!window.chrome?.runtime?.sendMessage) {
          resolve({ ok: false });
          return;
        }

        try {
          window.chrome.runtime.sendMessage(
            extensionId,
            {
              type: "ONUSLY_AUTH_HANDOFF",
              token,
              user: currentUser,
              baseUrl: API_BASE_URL,
            },
            (response) => {
              if (window.chrome.runtime.lastError || !response?.success) {
                resolve({
                  ok: false,
                  error: window.chrome.runtime.lastError?.message || response?.error,
                });
              } else {
                resolve({
                  ok: true,
                  targetsCount: response.targetsCount ?? targetsCount,
                  version: response.version ?? blocklistVersion,
                });
              }
            }
          );
        } catch (err) {
          resolve({ ok: false, error: err.message });
        }
      });
    };

    // Helper to send handoff via window content script bridge
    const sendViaBridge = () => {
      return new Promise((resolve) => {
        const timeout = setTimeout(() => {
          window.removeEventListener("message", onBridgeResponse);
          resolve({ ok: false, error: "timeout" });
        }, 3500);

        function onBridgeResponse(e) {
          if (e.source !== window || !e.data || typeof e.data !== "object") return;
          if (e.data.type === "ONUSLY_AUTH_HANDOFF_RESPONSE") {
            clearTimeout(timeout);
            window.removeEventListener("message", onBridgeResponse);
            if (e.data.response?.success) {
              resolve({
                ok: true,
                targetsCount: e.data.response.targetsCount ?? targetsCount,
                version: e.data.response.version ?? blocklistVersion,
              });
            } else {
              resolve({
                ok: false,
                error: e.data.error || e.data.response?.error,
              });
            }
          }
        }

        window.addEventListener("message", onBridgeResponse);
        window.postMessage(
          {
            type: "ONUSLY_AUTH_HANDOFF",
            token,
            user: currentUser,
            baseUrl: API_BASE_URL,
          },
          "*"
        );
      });
    };

    let result;
    try {
      const [runtimeRes, bridgeRes] = await Promise.all([sendViaRuntime(), sendViaBridge()]);
      result = runtimeRes.ok ? runtimeRes : bridgeRes.ok ? bridgeRes : null;
    } catch {
      result = null;
    }

    if (result && result.ok) {
      // Step 3: Successfully paired! Persist connection and update UI state
      const finalCount = result.targetsCount ?? targetsCount;
      const finalVersion = result.version ?? blocklistVersion;

      const connectionRecord = {
        connected: true,
        version: finalVersion,
        targetsCount: finalCount,
        connectedAt: Date.now(),
      };

      saveStoredConnection(userId, connectionRecord);
      setStatus("connected");
      setExtInfo(connectionRecord);
      setLoading(false);

      // Notify extension sync so all open tabs and background workers align
      notifyExtensionSync();

      setMessage(`✅ Extension connected! Enforcing ${finalCount} active target(s). Distractions will be blocked immediately.`);
      setMessageType("success");
      setTimeout(() => setMessage(""), 6000);
    } else {
      // Extension failed to answer handoff
      clearStoredConnection(userId);
      setStatus("disconnected");
      setExtInfo(null);
      setLoading(false);

      setMessage("⚠️ Extension not responding. If installed, open chrome://extensions, click 🔄 Reload on the ONUSLY extension card, refresh this page (F5), and click Connect Extension again.");
      setMessageType("error");
    }
  }, [extensionId, userId]);

  // Re-sync Action
  const handleResync = useCallback(async () => {
    setLoading(true);
    setMessage("");

    try {
      const blocklist = await getBlocklist().catch(() => ({ targets: [] }));
      const count = Array.isArray(blocklist?.targets) ? blocklist.targets.length : 0;
      const version = blocklist?.version || "active";

      // Forward sync request to extension via both runtime and bridge
      if (window.chrome?.runtime?.sendMessage) {
        try {
          window.chrome.runtime.sendMessage(extensionId, { type: "ONUSLY_SYNC" }, () => {});
        } catch {
          // ignore
        }
      }
      window.postMessage({ type: "ONUSLY_SYNC" }, "*");

      notifyExtensionSync();

      const updated = {
        connected: true,
        version,
        targetsCount: count,
        connectedAt: Date.now(),
      };

      saveStoredConnection(userId, updated);
      setExtInfo(updated);
      setMessage(`✅ Blocklist re-synchronized (${count} active target(s) locked).`);
      setMessageType("success");
      setTimeout(() => setMessage(""), 5000);
    } catch (err) {
      setMessage(err.message || "Failed to re-sync blocklist.");
      setMessageType("error");
    } finally {
      setLoading(false);
    }
  }, [extensionId, userId]);

  // Disconnect Action
  const handleDisconnect = useCallback(async () => {
    // Check if there are active goals enforcing restrictions
    let hasActiveGoals = false;
    try {
      const blocklist = await getBlocklist();
      if (blocklist?.targets && blocklist.targets.length > 0) {
        hasActiveGoals = blocklist.targets.some((t) => {
          const s = (t.status || "").toLowerCase().trim();
          return s === "active" || s === "proof_submitted";
        });
      }
    } catch {
      hasActiveGoals = (extInfo?.targetsCount || 0) > 0;
    }

    if (hasActiveGoals) {
      setMessage("⚠️ Cannot disconnect extension while goals are in progress! Complete or verify your active goals first.");
      setMessageType("error");
      setTimeout(() => setMessage(""), 5000);
      return;
    }

    clearStoredConnection(userId);

    // Forward disconnect signal to extension runtime and bridge
    if (window.chrome?.runtime?.sendMessage) {
      try {
        window.chrome.runtime.sendMessage(extensionId, { type: "ONUSLY_DISCONNECT" }, () => {});
      } catch {
        // ignore
      }
    }
    window.postMessage({ type: "ONUSLY_DISCONNECT" }, "*");

    setStatus("disconnected");
    setExtInfo(null);
    setMessage("Extension disconnected for this account.");
    setMessageType("info");
    setTimeout(() => setMessage(""), 4500);
  }, [extensionId, userId, extInfo]);

  const handleSaveId = (newId) => {
    const trimmed = (newId || "").trim();
    setExtensionId(trimmed || DEFAULT_EXTENSION_ID);
    localStorage.setItem("onusly_extension_id", trimmed || DEFAULT_EXTENSION_ID);
    setShowConfig(false);
  };

  const handleCopyToken = () => {
    const token = getToken();
    if (!token) {
      setMessage("Please sign in first.");
      setMessageType("error");
      return;
    }
    navigator.clipboard.writeText(token);
    setCopiedToken(true);
    setTimeout(() => setCopiedToken(false), 2500);
  };

  return (
    <div className="rounded-2xl border border-[#2A2A2A] bg-[#141414] p-5 shadow-sm">
      <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-center">
        <div className="flex items-center gap-3.5">
          <div
            className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border ${
              status === "connected"
                ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-400"
                : "border-indigo-500/40 bg-indigo-500/10 text-indigo-400"
            }`}
          >
            <Shield size={22} />
          </div>

          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-bold text-white">Browser Extension Enforcement</h3>
              {status === "connected" ? (
                <span className="flex items-center gap-1.5 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2.5 py-0.5 text-[10px] font-bold text-emerald-400">
                  <span className="h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse" />
                  Connected ({extInfo?.version || "active"})
                </span>
              ) : (
                <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2.5 py-0.5 text-[10px] font-bold text-amber-400">
                  Ready to Pair
                </span>
              )}
            </div>
            <p className="mt-0.5 text-xs text-[#A3A3A3]">
              {status === "connected"
                ? `Enforcing ${extInfo?.targetsCount ?? 0} active accountability target(s) across desktop browser tabs.`
                : "Link the ONUSLY extension to enforce goal website restrictions and block distractions."}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2.5">
          {status === "connected" ? (
            <>
              <button
                type="button"
                onClick={handleResync}
                disabled={loading}
                className="flex items-center gap-1.5 rounded-xl border border-neutral-700 bg-neutral-800/80 px-3.5 py-2 text-xs font-semibold text-neutral-200 transition-colors hover:bg-neutral-700 hover:text-white disabled:opacity-50"
                title="Re-sync blocklist with extension"
              >
                <RefreshCw size={13} className={loading ? "animate-spin" : ""} />
                <span>Re-sync</span>
              </button>
              <button
                type="button"
                onClick={handleDisconnect}
                className={`flex items-center gap-1.5 rounded-xl border px-3.5 py-2 text-xs font-semibold transition-colors ${
                  (extInfo?.targetsCount || 0) > 0
                    ? "border-amber-900/30 bg-amber-950/20 text-amber-400 hover:bg-amber-900/40"
                    : "border-red-900/30 bg-red-950/20 text-red-400 hover:bg-red-900/40"
                }`}
                title={
                  (extInfo?.targetsCount || 0) > 0
                    ? "Disconnection is locked while goals are in progress"
                    : "Disconnect extension"
                }
              >
                {(extInfo?.targetsCount || 0) > 0 ? (
                  <>
                    <span>🔒</span>
                    <span>Locked (Active Goals)</span>
                  </>
                ) : (
                  <>
                    <Unlink size={13} />
                    <span>Disconnect</span>
                  </>
                )}
              </button>
            </>
          ) : (
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => setShowInstallModal(true)}
                className="flex items-center gap-1.5 rounded-xl border border-neutral-700 bg-neutral-800/90 px-3 py-2 text-xs font-semibold text-neutral-200 transition-colors hover:bg-neutral-700 hover:text-white"
                title="Download Extension ZIP & Setup Guide"
              >
                <Download size={13} />
                <span>Download Extension</span>
              </button>

              <button
                type="button"
                onClick={handleConnect}
                disabled={loading}
                className="flex items-center gap-2 rounded-xl bg-indigo-600 px-4 py-2 text-xs font-bold text-white shadow-sm transition-all hover:bg-indigo-500 disabled:opacity-50"
              >
                {loading ? <RefreshCw size={14} className="animate-spin" /> : <Shield size={14} />}
                <span>{loading ? "Connecting..." : "Connect Extension"}</span>
              </button>
            </div>
          )}

          <button
            type="button"
            onClick={() => setShowConfig(!showConfig)}
            className="rounded-xl border border-neutral-800 p-2 text-xs text-neutral-400 hover:text-white hover:bg-neutral-800/60 transition"
            title="Configure Extension Connection & Pairing Token"
          >
            ⚙️
          </button>
        </div>
      </div>

      {message && (
        <div
          className={`mt-3 flex items-start gap-2.5 rounded-xl border p-3 text-xs leading-relaxed ${
            messageType === "success"
              ? "border-emerald-500/40 bg-emerald-950/30 text-emerald-300"
              : messageType === "error"
              ? "border-rose-500/40 bg-rose-950/30 text-rose-300"
              : "border-neutral-800 bg-neutral-900/80 text-neutral-300"
          }`}
        >
          {messageType === "success" ? (
            <CheckCircle2 size={16} className="text-emerald-400 shrink-0 mt-0.5" />
          ) : (
            <AlertCircle size={16} className="text-amber-400 shrink-0 mt-0.5" />
          )}
          <div className="flex-1">
            <span>{message}</span>
          </div>
        </div>
      )}

      {showConfig && (
        <div className="mt-4 border-t border-neutral-800 pt-3.5 space-y-3">
          <div>
            <label className="block text-[11px] font-semibold text-neutral-400 mb-1">
              Extension ID (from chrome://extensions)
            </label>
            <div className="flex items-center gap-2">
              <input
                type="text"
                defaultValue={extensionId}
                onBlur={(e) => handleSaveId(e.target.value)}
                className="w-full max-w-md rounded-lg border border-neutral-700 bg-neutral-900 px-3 py-1.5 font-mono text-xs text-white focus:border-indigo-500 focus:outline-none"
                placeholder="Extension ID"
              />
              <button
                type="button"
                onClick={() => handleSaveId(DEFAULT_EXTENSION_ID)}
                className="rounded-lg border border-neutral-700 px-3 py-1.5 text-xs text-neutral-300 hover:bg-neutral-800"
              >
                Reset
              </button>
            </div>
          </div>

          <div className="rounded-xl border border-neutral-800 bg-neutral-900/60 p-3 text-xs">
            <div className="flex items-center justify-between">
              <div>
                <p className="font-semibold text-neutral-200">Alternative: Pair via Extension Popup</p>
                <p className="text-[11px] text-neutral-400 mt-0.5">
                  Click the ONUSLY extension icon in your browser toolbar, then click <strong>"⚡ Quick Connect via Tab"</strong> or paste the token below.
                </p>
              </div>
              <button
                type="button"
                onClick={handleCopyToken}
                className="flex items-center gap-1.5 rounded-lg border border-neutral-700 bg-neutral-800 px-3 py-1.5 text-xs font-semibold text-neutral-200 hover:bg-neutral-700 hover:text-white transition shrink-0 ml-3"
              >
                {copiedToken ? <Check size={13} className="text-emerald-400" /> : <Copy size={13} />}
                <span>{copiedToken ? "Copied!" : "Copy Token"}</span>
              </button>
            </div>
          </div>

          <div className="text-[11px] text-neutral-400 bg-neutral-950/40 p-2.5 rounded-lg border border-neutral-800/80 space-y-1">
            <p className="font-bold text-neutral-300">💡 Installation / Reloading Guide:</p>
            <p>1. Open <code className="text-neutral-200 bg-neutral-800 px-1 py-0.5 rounded">chrome://extensions</code> and enable <strong>Developer mode</strong>.</p>
            <p>2. If loaded unpacked, click the 🔄 <strong>Reload</strong> button on the ONUSLY card whenever code updates.</p>
            <p>3. Refresh this dashboard page (F5) and click <strong>Connect Extension</strong>.</p>
          </div>
        </div>
      )}

      {/* Extension Download & Installation Guide Modal */}
      {showInstallModal && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm"
          onClick={() => setShowInstallModal(false)}
        >
          <div
            className="w-full max-w-lg rounded-2xl border border-neutral-800 bg-[#161616] p-6 shadow-2xl space-y-5"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-start justify-between">
              <div className="flex items-center gap-3">
                <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-indigo-500/30 bg-indigo-500/10 text-indigo-400">
                  <FolderArchive size={20} />
                </div>
                <div>
                  <h3 className="text-base font-bold text-white">Download & Install ONUSLY Extension</h3>
                  <p className="text-xs text-neutral-400">Install directly in under 30 seconds</p>
                </div>
              </div>
              <button
                type="button"
                onClick={() => setShowInstallModal(false)}
                className="rounded-lg p-1 text-neutral-400 hover:text-white hover:bg-neutral-800 transition"
              >
                <X size={18} />
              </button>
            </div>

            <div className="space-y-3 text-xs text-neutral-300">
              <div className="rounded-xl border border-neutral-800 bg-neutral-900/60 p-3.5 space-y-2">
                <div className="flex items-center gap-2 text-white font-semibold">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-indigo-600 text-[11px] font-bold">1</span>
                  <span>Download the Extension Package</span>
                </div>
                <p className="text-neutral-400 pl-7 leading-relaxed">
                  Click the button below to download the latest packed ZIP archive:
                </p>
                <div className="pl-7 pt-1">
                  <a
                    href="/downloads/onusly-extension.zip"
                    download="onusly-extension.zip"
                    className="inline-flex items-center gap-2 rounded-xl bg-white px-4 py-2 text-xs font-bold text-black hover:bg-neutral-200 transition shadow-sm"
                  >
                    <Download size={14} />
                    <span>Download onusly-extension.zip</span>
                  </a>
                </div>
              </div>

              <div className="rounded-xl border border-neutral-800 bg-neutral-900/60 p-3.5 space-y-1.5">
                <div className="flex items-center gap-2 text-white font-semibold">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-indigo-600 text-[11px] font-bold">2</span>
                  <span>Extract the ZIP Archive</span>
                </div>
                <p className="text-neutral-400 pl-7 leading-relaxed">
                  Right-click the downloaded <code className="text-neutral-200 bg-neutral-800 px-1 py-0.5 rounded">onusly-extension.zip</code> file and click <strong>"Extract All"</strong> (or unzip it to a folder).
                </p>
              </div>

              <div className="rounded-xl border border-neutral-800 bg-neutral-900/60 p-3.5 space-y-2">
                <div className="flex items-center gap-2 text-white font-semibold">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-indigo-600 text-[11px] font-bold">3</span>
                  <span>Load in Your Browser (Windows & Linux)</span>
                </div>
                <div className="text-neutral-400 pl-7 space-y-2 leading-relaxed">
                  <div className="space-y-0.5">
                    <p className="font-semibold text-neutral-200">Chrome / Edge / Brave / Opera / Vivaldi:</p>
                    <p>• In your URL bar, open <code className="text-neutral-200 bg-neutral-800 px-1 py-0.5 rounded">chrome://extensions</code> (or <code className="text-neutral-200 bg-neutral-800 px-1 py-0.5 rounded">edge://extensions</code>, <code className="text-neutral-200 bg-neutral-800 px-1 py-0.5 rounded">brave://extensions</code>).</p>
                    <p>• Turn ON <strong>Developer mode</strong> (toggle in top-right).</p>
                    <p>• Click <strong>"Load unpacked"</strong> and select the extracted folder.</p>
                  </div>
                  <div className="pt-2 border-t border-neutral-800/80 space-y-0.5">
                    <p className="font-semibold text-neutral-200">Mozilla Firefox (Linux & Windows):</p>
                    <p>• In Firefox URL bar, enter <code className="text-neutral-200 bg-neutral-800 px-1 py-0.5 rounded">about:debugging#/runtime/this-firefox</code>.</p>
                    <p>• Click <strong>"Load Temporary Add-on..."</strong> and select <code className="text-neutral-200 bg-neutral-800 px-1 py-0.5 rounded">manifest.json</code> from the extracted folder.</p>
                  </div>
                </div>
              </div>
            </div>

            <div className="flex items-center justify-between border-t border-neutral-800 pt-4">
              <button
                type="button"
                onClick={() => setShowInstallModal(false)}
                className="rounded-xl border border-neutral-700 px-4 py-2 text-xs font-semibold text-neutral-300 hover:bg-neutral-800 transition"
              >
                Close
              </button>

              <button
                type="button"
                onClick={async () => {
                  await handleConnect();
                  setShowInstallModal(false);
                }}
                disabled={loading}
                className="flex items-center gap-2 rounded-xl bg-indigo-600 px-5 py-2 text-xs font-bold text-white shadow-sm hover:bg-indigo-500 transition disabled:opacity-50"
              >
                <Shield size={14} />
                <span>I've Loaded It — Connect Now</span>
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
