import { useState, useEffect, useCallback } from "react";
import {
  Laptop,
  Smartphone,
  Globe,
  ShieldCheck,
  AlertTriangle,
  RefreshCw,
  KeyRound,
  CheckCircle2,
  Server,
  Trash2,
  X,
  Ban,
} from "lucide-react";
import { getDevicesCoverage, confirmDevicePairing, revokeDevice } from "../lib/api";

function formatRelativeTime(dateStr) {
  if (!dateStr) return "Never";
  try {
    const date = new Date(dateStr);
    const now = new Date();
    const diffSec = Math.floor((now - date) / 1000);
    if (diffSec < 45) return "Just now";
    if (diffSec < 90) return "1 min ago";
    if (diffSec < 3600) return `${Math.floor(diffSec / 60)} mins ago`;
    if (diffSec < 86400) return `${Math.floor(diffSec / 3600)} hours ago`;
    return `${Math.floor(diffSec / 86400)} days ago`;
  } catch {
    return dateStr;
  }
}

function getLifecycleBadge(d) {
  const status = (d.status || "").toUpperCase();
  const health = (d.healthStatus || "").toUpperCase();

  if (status === "REVOKED" || health === "DEVICE_REVOKED") {
    return (
      <span className="inline-flex items-center gap-1 rounded-full border border-red-500/30 bg-red-500/10 px-2 py-0.5 text-[10px] font-bold text-red-400">
        <Ban size={10} />
        Revoked
      </span>
    );
  }
  if (status === "AUTHENTICATION_REQUIRED" || status === "AUTH_REQUIRED" || health === "AUTHENTICATION_REQUIRED") {
    return (
      <span className="inline-flex items-center gap-1 rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[10px] font-bold text-amber-300">
        <AlertTriangle size={10} />
        Auth Required
      </span>
    );
  }
  if (health === "DNS_UNAVAILABLE") {
    return (
      <div>
        <span className="inline-flex items-center gap-1 rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[10px] font-bold text-amber-300">
          <AlertTriangle size={10} />
          DNS Unavailable
        </span>
        <div className="text-[10px] text-amber-400/80 mt-0.5">Loopback DNS resolver stopped</div>
      </div>
    );
  }
  if (health === "APP_ENFORCEMENT_DEGRADED") {
    return (
      <div>
        <span className="inline-flex items-center gap-1 rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[10px] font-bold text-amber-300">
          <AlertTriangle size={10} />
          App Monitor Degraded
        </span>
      </div>
    );
  }
  if (!d.enforcing || (d.issues && d.issues.length > 0) || health === "DEGRADED" || health === "POLICY_MISSING" || health === "POLICY_STALE") {
    return (
      <div>
        <span className="inline-flex items-center gap-1 rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[10px] font-bold text-amber-300">
          <AlertTriangle size={10} />
          Degraded
        </span>
        {d.issues && d.issues.length > 0 && (
          <div className="text-[10px] text-amber-400/80 mt-0.5">
            {d.issues.join(", ").replace(/_/g, " ")}
          </div>
        )}
      </div>
    );
  }
  const isRecent = d.lastHeartbeatAt && Date.now() - new Date(d.lastHeartbeatAt).getTime() < 5 * 60 * 1000;
  if (!isRecent) {
    return (
      <span className="inline-flex items-center gap-1 rounded-full border border-neutral-700 bg-neutral-800 px-2 py-0.5 text-[10px] font-bold text-neutral-400">
        <span className="h-1.5 w-1.5 rounded-full bg-neutral-500" />
        Offline
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 rounded-full border border-white/20 bg-white/10 px-2 py-0.5 text-[10px] font-bold text-white">
      <span className="h-1.5 w-1.5 rounded-full bg-white" />
      Online
    </span>
  );
}

export default function DeviceCoverageSection() {
  const [data, setData] = useState({ userId: "", totalCount: 0, enforcing: false, devices: [] });
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [pairCode, setPairCode] = useState("");
  const [pairingLoading, setPairingLoading] = useState(false);
  const [pairingMsg, setPairingMsg] = useState({ text: "", type: "" });
  const [deviceToRevoke, setDeviceToRevoke] = useState(null);
  const [revoking, setRevoking] = useState(false);
  const [revokeError, setRevokeError] = useState("");

  const loadCoverage = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    try {
      const res = await getDevicesCoverage();
      setData(res);
    } catch (err) {
      console.error("[DeviceCoverage] Failed to load devices:", err);
    } finally {
      setLoading(false);
      if (isRefresh) setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    let isMounted = true;
    const fetchInitial = async () => {
      try {
        const res = await getDevicesCoverage();
        if (isMounted) {
          setData(res);
          setLoading(false);
        }
      } catch (err) {
        console.error("[DeviceCoverage] Failed to load devices:", err);
        if (isMounted) {
          setLoading(false);
        }
      }
    };
    fetchInitial();
    const interval = setInterval(() => {
      fetchInitial();
    }, 30000);
    return () => {
      isMounted = false;
      clearInterval(interval);
    };
  }, []);

  const handleConfirmPair = async (e) => {
    e.preventDefault();
    const cleanCode = pairCode.trim().toUpperCase();
    if (!cleanCode || cleanCode.length !== 6) {
      setPairingMsg({ text: "Please enter a valid 6-character code.", type: "error" });
      return;
    }

    setPairingLoading(true);
    setPairingMsg({ text: "", type: "" });
    try {
      await confirmDevicePairing(cleanCode);
      setPairingMsg({ text: "Device paired successfully! Synchronizing...", type: "success" });
      setPairCode("");
      await loadCoverage(true);
    } catch (err) {
      setPairingMsg({ text: err.message || "Failed to pair device.", type: "error" });
    } finally {
      setPairingLoading(false);
    }
  };

  const handleConfirmRevoke = async () => {
    if (!deviceToRevoke) return;
    setRevoking(true);
    setRevokeError("");
    try {
      await revokeDevice(deviceToRevoke.deviceId, "user_revoked");
      const shortId = deviceToRevoke.deviceId.slice(0, 8);
      setDeviceToRevoke(null);
      setPairingMsg({ text: `Device (${shortId}...) was successfully revoked.`, type: "success" });
      await loadCoverage(true);
    } catch (err) {
      setRevokeError(err.message || "Failed to revoke device.");
    } finally {
      setRevoking(false);
    }
  };

  // Exclude revoked devices from active fleet enforcement calculation
  const activeDevices = (data.devices || []).filter(
    (d) => (d.status || "").toUpperCase() !== "REVOKED"
  );
  const allEnforcing =
    activeDevices.length > 0 &&
    activeDevices.every((d) => d.enforcing && (!d.issues || d.issues.length === 0));

  return (
    <div className="card-stat rounded-2xl p-6 shadow-sm border border-white/[0.08] transition-all space-y-5">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl border border-white/20 bg-white/10 text-white shadow-sm">
            <Server size={22} />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-base font-bold text-white tracking-tight">Active Device & Browser Coverage</h3>
              {data.devices.length > 0 && (
                <span
                  className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-[10px] font-bold ${
                    allEnforcing
                      ? "border-white/20 bg-white/10 text-white"
                      : "border-amber-500/30 bg-amber-500/10 text-amber-300"
                  }`}
                >
                  {allEnforcing ? (
                    <>
                      <ShieldCheck size={11} className="text-white" />
                      All Enforcing
                    </>
                  ) : (
                    <>
                      <AlertTriangle size={11} className="text-amber-400" />
                      Issues Detected
                    </>
                  )}
                </span>
              )}
            </div>
            <p className="text-xs text-[#A1A1AA] mt-0.5">
              Live heartbeat telemetry across registered desktop agents, mobile clients, and connected browsers.
            </p>
          </div>
        </div>

        <button
          type="button"
          onClick={() => loadCoverage(true)}
          disabled={refreshing}
          className="btn-premium self-start sm:self-auto flex items-center gap-1.5 rounded-xl border border-white/10 bg-white/5 px-3.5 py-1.5 text-xs font-semibold text-neutral-300 hover:text-white hover:bg-white/10 transition disabled:opacity-50"
        >
          <RefreshCw size={12} className={refreshing ? "animate-spin" : ""} />
          <span>Refresh</span>
        </button>
      </div>

      {/* Quick Pair Desktop Agent Input */}
      <div className="rounded-xl border border-white/10 bg-white/[0.02] p-4 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h4 className="text-xs font-bold text-white flex items-center gap-1.5">
            <KeyRound size={14} className="text-white" />
            Pair ONUSLY Desktop Agent
          </h4>
          <p className="text-[11px] text-[#A1A1AA] mt-0.5">
            Enter the 6-character code displayed by the desktop agent to securely authorize all browsers on that machine.
          </p>
        </div>

        <form onSubmit={handleConfirmPair} className="flex items-center gap-2">
          <input
            type="text"
            maxLength={6}
            value={pairCode}
            onChange={(e) => setPairCode(e.target.value.toUpperCase())}
            placeholder="e.g. U8GCM4"
            className="w-32 uppercase tracking-widest text-center font-mono rounded-lg border border-white/15 bg-black/60 px-3 py-1.5 text-xs font-bold text-white placeholder-neutral-600 focus:border-white focus:outline-none"
          />
          <button
            type="submit"
            disabled={pairingLoading || pairCode.trim().length !== 6}
            className="btn-premium inline-flex items-center gap-1.5 rounded-lg bg-white px-3.5 py-1.5 text-xs font-bold text-black hover:bg-neutral-200 transition disabled:opacity-40"
          >
            {pairingLoading ? <RefreshCw size={12} className="animate-spin text-black" /> : <CheckCircle2 size={12} />}
            <span>Pair Device</span>
          </button>
        </form>
      </div>

      {pairingMsg.text && (
        <div
          className={`p-2.5 rounded-lg text-xs font-semibold flex items-center gap-2 border ${
            pairingMsg.type === "success"
              ? "border-white/20 bg-white/10 text-white"
              : "border-red-500/20 bg-red-500/10 text-red-300"
          }`}
        >
          {pairingMsg.type === "success" ? <CheckCircle2 size={14} /> : <AlertTriangle size={14} />}
          <span>{pairingMsg.text}</span>
        </div>
      )}

      {/* Devices List Table/Cards */}
      {loading ? (
        <div className="py-8 text-center text-xs text-neutral-500">
          <RefreshCw size={18} className="animate-spin mx-auto mb-2 opacity-50" />
          Loading connected devices...
        </div>
      ) : data.devices.length === 0 ? (
        <div className="rounded-xl border border-dashed border-white/10 p-6 text-center text-xs text-neutral-400">
          <p className="font-semibold text-neutral-300">No active devices registered yet</p>
          <p className="mt-1 text-[11px] text-neutral-500">
            Install ONUSLY Desktop on Linux/Windows or sign into the Android app to begin automated synchronized enforcement.
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead>
              <tr className="border-b border-white/[0.08] text-[11px] font-semibold text-[#A1A1AA]">
                <th className="pb-2.5">Device & Platform</th>
                <th className="pb-2.5">Client Type</th>
                <th className="pb-2.5">Browser</th>
                <th className="pb-2.5">Version</th>
                <th className="pb-2.5">Blocklist Version</th>
                <th className="pb-2.5">Last Heartbeat</th>
                <th className="pb-2.5">Status</th>
                <th className="pb-2.5 text-right">Action</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.05]">
              {data.devices.map((d) => {
                const isRevoked = (d.status || "").toUpperCase() === "REVOKED";
                return (
                  <tr key={d.deviceId} className="hover:bg-white/[0.02] transition">
                    <td className="py-3 font-medium text-white flex items-center gap-2">
                      {d.platform === "android" ? (
                        <Smartphone size={15} className="text-neutral-400 shrink-0" />
                      ) : (
                        <Laptop size={15} className="text-neutral-400 shrink-0" />
                      )}
                      <div>
                        <div className="capitalize">{d.platform}</div>
                        <div className="font-mono text-[10px] text-neutral-500">{d.deviceId.slice(0, 8)}...</div>
                      </div>
                    </td>
                    <td className="py-3 text-neutral-300 capitalize">{d.clientType.replace("-", " ")}</td>
                    <td className="py-3 text-neutral-300">
                      {d.browser ? (
                        <span className="inline-flex items-center gap-1 capitalize">
                          <Globe size={12} className="text-neutral-400" />
                          {d.browser}
                        </span>
                      ) : (
                        "—"
                      )}
                    </td>
                    <td className="py-3 text-neutral-400 font-mono text-[11px]">v{d.clientVersion}</td>
                    <td className="py-3 text-neutral-400 font-mono text-[11px]">{d.blocklistVersion || "—"}</td>
                    <td className="py-3 text-neutral-400">{formatRelativeTime(d.lastHeartbeatAt)}</td>
                    <td className="py-3">
                      {getLifecycleBadge(d)}
                    </td>
                    <td className="py-3 text-right">
                      {isRevoked ? (
                        <span className="text-[11px] text-neutral-500 font-medium">Decommissioned</span>
                      ) : (
                        <button
                          type="button"
                          onClick={() => {
                            setRevokeError("");
                            setDeviceToRevoke(d);
                          }}
                          className="inline-flex items-center gap-1 rounded-lg border border-red-500/30 bg-red-500/10 px-2.5 py-1 text-xs font-semibold text-red-400 hover:bg-red-500/20 hover:text-red-300 transition"
                        >
                          <Trash2 size={12} />
                          <span>Revoke</span>
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* Revoke Confirmation Modal */}
      {deviceToRevoke && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4">
          <div className="w-full max-w-md rounded-2xl border border-white/10 bg-neutral-900 p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-base font-bold text-white flex items-center gap-2">
                <AlertTriangle size={18} className="text-red-400" />
                Revoke Device Authorization
              </h3>
              <button
                type="button"
                onClick={() => setDeviceToRevoke(null)}
                disabled={revoking}
                className="text-neutral-400 hover:text-white transition"
              >
                <X size={18} />
              </button>
            </div>

            <p className="text-xs text-neutral-300 leading-relaxed">
              Are you sure you want to revoke authorization for this device?
            </p>

            <div className="rounded-xl border border-white/10 bg-black/40 p-3.5 space-y-2 text-xs">
              <div className="flex justify-between">
                <span className="text-neutral-400">Device ID:</span>
                <span className="font-mono text-white">{deviceToRevoke.deviceId}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-neutral-400">Platform:</span>
                <span className="text-white capitalize">{deviceToRevoke.platform}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-neutral-400">Client Type:</span>
                <span className="text-white capitalize">{deviceToRevoke.clientType}</span>
              </div>
            </div>

            <div className="rounded-xl border border-red-500/20 bg-red-500/10 p-3 text-xs text-red-300 space-y-1">
              <p className="font-bold">Security Notice:</p>
              <p className="text-[11px] leading-relaxed">
                Revoking this device will permanently invalidate its credentials, disable automated enforcement, and require re-pairing to reconnect.
              </p>
            </div>

            {revokeError && (
              <div className="rounded-lg border border-red-500/30 bg-red-500/20 p-2.5 text-xs text-red-200">
                {revokeError}
              </div>
            )}

            <div className="flex items-center justify-end gap-3 pt-2">
              <button
                type="button"
                onClick={() => setDeviceToRevoke(null)}
                disabled={revoking}
                className="rounded-xl border border-white/10 px-4 py-2 text-xs font-semibold text-neutral-300 hover:text-white hover:bg-white/5 transition disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleConfirmRevoke}
                disabled={revoking}
                className="inline-flex items-center gap-1.5 rounded-xl bg-red-600 px-4 py-2 text-xs font-bold text-white hover:bg-red-500 transition disabled:opacity-50"
              >
                {revoking ? <RefreshCw size={13} className="animate-spin text-white" /> : <Ban size={13} />}
                <span>{revoking ? "Revoking..." : "Confirm Revocation"}</span>
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
