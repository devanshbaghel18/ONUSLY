import { useState, useEffect } from "react";
import { Shield, AlertCircle, RefreshCw, Unlink } from "lucide-react";
import { getToken, getUser } from "../lib/auth";

// Deterministic Extension ID from manifest.json key
export const DEFAULT_EXTENSION_ID = "gkichkblphmdbojdjnonljlegnmjcnhg";

export default function ConnectExtension() {
  const [extensionId, setExtensionId] = useState(() => {
    return localStorage.getItem("onusly_extension_id") || DEFAULT_EXTENSION_ID;
  });
  const [showConfig, setShowConfig] = useState(false);
  const [status, setStatus] = useState(() => {
    if (typeof window !== "undefined" && !window.chrome?.runtime?.sendMessage) {
      return "not_installed";
    }
    return "unknown";
  });
  const [extInfo, setExtInfo] = useState(null);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");

  useEffect(() => {
    if (!window.chrome?.runtime?.sendMessage) {
      return;
    }

    let isMounted = true;
    try {
      window.chrome.runtime.sendMessage(
        extensionId,
        { type: "ONUSLY_PING" },
        (response) => {
          if (!isMounted) return;
          if (window.chrome.runtime.lastError) {
            setStatus("not_installed");
            setExtInfo(null);
            return;
          }

          if (response?.status === "ok") {
            if (response.connected) {
              setStatus("connected");
              setExtInfo(response);
            } else {
              setStatus("disconnected");
              setExtInfo(null);
            }
          } else {
            setStatus("not_installed");
          }
        }
      );
    } catch {
      // Chrome runtime error
    }

    return () => {
      isMounted = false;
    };
  }, [extensionId]);

  const handleConnect = () => {
    const token = getToken();
    const user = getUser();

    if (!token) {
      setMessage("Please sign in to ONUSLY first.");
      return;
    }

    if (!window.chrome?.runtime?.sendMessage) {
      setMessage("Chrome extension runtime not detected. Open this dashboard in Chrome/Brave/Edge.");
      return;
    }

    setLoading(true);
    setMessage("");

    try {
      window.chrome.runtime.sendMessage(
        extensionId,
        {
          type: "ONUSLY_AUTH_HANDOFF",
          token,
          user,
        },
        (response) => {
          setLoading(false);
          if (window.chrome.runtime.lastError) {
            setStatus("not_installed");
            setMessage("Extension not found. Load the unpacked extension from /extension in chrome://extensions.");
            return;
          }

          if (response?.success) {
            setStatus("connected");
            setExtInfo(response);
            setMessage(`Connected! Synchronized ${response.targetsCount || 0} active blocklist target(s).`);
          } else {
            setMessage(response?.error || "Connection failed.");
          }
        }
      );
    } catch (err) {
      setLoading(false);
      setMessage(err.message || "Failed to communicate with extension.");
    }
  };

  const handleDisconnect = () => {
    if (!window.chrome?.runtime?.sendMessage) return;

    window.chrome.runtime.sendMessage(
      extensionId,
      { type: "ONUSLY_DISCONNECT" },
      () => {
        setStatus("disconnected");
        setExtInfo(null);
        setMessage("Extension disconnected.");
      }
    );
  };

  const handleSaveId = (newId) => {
    const trimmed = (newId || "").trim();
    setExtensionId(trimmed || DEFAULT_EXTENSION_ID);
    localStorage.setItem("onusly_extension_id", trimmed || DEFAULT_EXTENSION_ID);
    setShowConfig(false);
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
                <span className="flex items-center gap-1 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2 py-0.5 text-[10px] font-bold text-emerald-400">
                  <span className="h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse" />
                  Connected ({extInfo?.version || "active"})
                </span>
              ) : status === "disconnected" ? (
                <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[10px] font-bold text-amber-400">
                  Ready to Pair
                </span>
              ) : (
                <span className="rounded-full border border-neutral-700 bg-neutral-800 px-2 py-0.5 text-[10px] font-medium text-neutral-400">
                  Not Installed
                </span>
              )}
            </div>
            <p className="mt-0.5 text-xs text-[#A3A3A3]">
              {status === "connected"
                ? `Enforcing ${extInfo?.targetsCount ?? 0} active accountability target(s) across desktop browser tabs.`
                : "Link the ONUSLY extension to enforce goal website restrictions on your computer."}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2.5">
          {status === "connected" ? (
            <>
              <button
                type="button"
                onClick={handleConnect}
                disabled={loading}
                className="flex items-center gap-1.5 rounded-xl border border-neutral-700 bg-neutral-800/80 px-3.5 py-2 text-xs font-semibold text-neutral-200 transition-colors hover:bg-neutral-700"
                title="Re-sync blocklist with extension"
              >
                <RefreshCw size={13} className={loading ? "animate-spin" : ""} />
                Re-sync
              </button>
              <button
                type="button"
                onClick={handleDisconnect}
                className="flex items-center gap-1.5 rounded-xl border border-red-900/30 bg-red-950/20 px-3.5 py-2 text-xs font-semibold text-red-400 transition-colors hover:bg-red-900/40"
              >
                <Unlink size={13} />
                Disconnect
              </button>
            </>
          ) : (
            <button
              type="button"
              onClick={handleConnect}
              disabled={loading}
              className="flex items-center gap-2 rounded-xl bg-indigo-600 px-4 py-2 text-xs font-bold text-white shadow-sm transition-all hover:bg-indigo-500 disabled:opacity-50"
            >
              {loading ? <RefreshCw size={14} className="animate-spin" /> : <Shield size={14} />}
              Connect Extension
            </button>
          )}

          <button
            type="button"
            onClick={() => setShowConfig(!showConfig)}
            className="rounded-xl border border-neutral-800 p-2 text-xs text-neutral-400 hover:text-white"
            title="Configure Extension ID"
          >
            ⚙️
          </button>
        </div>
      </div>

      {message && (
        <div className="mt-3 flex items-center gap-2 rounded-xl border border-neutral-800 bg-neutral-900/80 px-3.5 py-2 text-xs text-neutral-300">
          <AlertCircle size={14} className="text-amber-400 shrink-0" />
          <span>{message}</span>
        </div>
      )}

      {showConfig && (
        <div className="mt-4 border-t border-neutral-800 pt-3">
          <label className="block text-[11px] font-semibold text-neutral-400">
            Extension ID (from chrome://extensions)
          </label>
          <div className="mt-1 flex items-center gap-2">
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
      )}
    </div>
  );
}
