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
  Shield,
  Power,
  Download,
} from "lucide-react";
import {
  getDevicesCoverage,
  confirmDevicePairing,
  revokeDevice,
  getDesktopAgentStatus,
  toggleDesktopDNS,
  syncDesktopAgent,
  autoPairDesktopAgent,
  reconnectDesktopAgent,
} from "../lib/api";

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

  // Machine Desktop Agent state
  const [desktopAgent, setDesktopAgent] = useState({ available: false, loading: true, status: null, error: "" });
  const [dnsActionLoading, setDnsActionLoading] = useState(false);
  const [syncActionLoading, setSyncActionLoading] = useState(false);
  const [autoPairLoading, setAutoPairLoading] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const [desktopActionMsg, setDesktopActionMsg] = useState({ text: "", type: "" });

  const loadDesktopAgent = useCallback(async () => {
    try {
      const res = await getDesktopAgentStatus();
      if (res && res.available) {
        setDesktopAgent({ available: true, loading: false, status: res.status, error: "" });
      } else {
        setDesktopAgent({ available: false, loading: false, status: null, error: res?.error || "Agent unreachable" });
      }
    } catch (err) {
      setDesktopAgent({ available: false, loading: false, status: null, error: err.message });
    }
  }, []);

  const loadCoverage = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    try {
      const [res] = await Promise.all([getDevicesCoverage(), loadDesktopAgent()]);
      if (res) {
        setData({
          ...res,
          devices: Array.isArray(res.devices) ? res.devices : [],
        });
      }
    } catch (err) {
      console.error("[DeviceCoverage] Failed to load devices:", err);
    } finally {
      setLoading(false);
      if (isRefresh) setRefreshing(false);
    }
  }, [loadDesktopAgent]);

  useEffect(() => {
    let isMounted = true;
    const fetchInitial = async () => {
      try {
        const [res] = await Promise.all([
          getDevicesCoverage(),
          loadDesktopAgent(),
        ]);
        if (isMounted && res) {
          setData({
            ...res,
            devices: Array.isArray(res.devices) ? res.devices : [],
          });
        }
      } catch (err) {
        console.error("[DeviceCoverage] Failed to load devices:", err);
      } finally {
        if (isMounted) {
          setLoading(false);
        }
      }
    };
    fetchInitial();
    const interval = setInterval(fetchInitial, 8000);
    return () => {
      isMounted = false;
      clearInterval(interval);
    };
  }, [loadDesktopAgent]);

  // Auto-pair from URL query param ?pairCode=XXXXXX (used by OnuslySetup.exe)
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get("pairCode");
    if (code && code.trim().length === 6) {
      const clean = code.trim().toUpperCase();
      setPairCode(clean);
      setPairingLoading(true);
      confirmDevicePairing(clean)
        .then(() => {
          setPairingMsg({ text: "Device paired successfully! Synchronizing...", type: "success" });
          loadCoverage(true);
        })
        .catch((err) => {
          setPairingMsg({ text: err.message || "Failed to pair device.", type: "error" });
        })
        .finally(() => {
          setPairingLoading(false);
        });
    }
  }, [loadCoverage]);

  const handleToggleDNS = async (currentActive) => {
    setDnsActionLoading(true);
    setDesktopActionMsg({ text: "", type: "" });
    try {
      const nextState = !currentActive;
      const res = await toggleDesktopDNS(nextState);
      if (res && res.success) {
        setDesktopAgent((prev) => ({ ...prev, status: res.status }));
        setDesktopActionMsg({
          text: nextState
            ? "Machine-wide DNS Shield activated (127.0.0.1)! Distraction domains are now blocked across all browsers."
            : "DNS Shield restored to standard upstream. Distraction domains unblocked machine-wide.",
          type: "success",
        });
        await loadCoverage(true);
      }
    } catch (err) {
      const isPermission = err.message && (err.message.includes("PermissionDenied") || err.message.includes("Access to a CIM resource") || err.message.includes("unsupported action") || err.message.includes("needs a one-time update"));
      if (isPermission) {
        setDesktopActionMsg({
          text: currentActive
            ? "To deactivate the shield, double-click 'ONUSLY_Shield_Toggle.exe' on your Desktop (or click '1-Click Switcher') and click 'Allow' in the Windows prompt."
            : "To activate the shield, double-click 'ONUSLY_Shield_Toggle.exe' on your Desktop (or click '1-Click Switcher') and click 'Allow' in the Windows prompt.",
          type: "warning",
        });
      } else {
        setDesktopActionMsg({ text: err.message || "Failed to toggle DNS shield", type: "error" });
      }
    } finally {
      setDnsActionLoading(false);
    }
  };

  const handleSyncDesktop = async () => {
    setSyncActionLoading(true);
    setDesktopActionMsg({ text: "", type: "" });
    try {
      const res = await syncDesktopAgent();
      if (res && res.success) {
        setDesktopAgent((prev) => ({ ...prev, status: res.status }));
        setDesktopActionMsg({
          text: `Machine policy updated successfully! ${res.status?.targetsCount || 0} active target(s) locked.`,
          type: "success",
        });
        await loadCoverage(true);
      }
    } catch (err) {
      setDesktopActionMsg({ text: err.message || "Failed to sync desktop policy", type: "error" });
    } finally {
      setSyncActionLoading(false);
    }
  };

  const handleAutoPairDesktop = async () => {
    setAutoPairLoading(true);
    setDesktopActionMsg({ text: "", type: "" });
    try {
      const res = await autoPairDesktopAgent();
      if (res && res.success) {
        setDesktopAgent((prev) => ({ ...prev, status: res.status }));
        setDesktopActionMsg({
          text: "Desktop agent automatically paired and synchronized with this account!",
          type: "success",
        });
        await loadCoverage(true);
      }
    } catch (err) {
      setDesktopActionMsg({ text: err.message || "Auto-pairing failed", type: "error" });
    } finally {
      setAutoPairLoading(false);
    }
  };

  const handleReconnectDesktop = async () => {
    setReconnecting(true);
    setDesktopActionMsg({ text: "", type: "" });
    try {
      const res = await reconnectDesktopAgent();
      if (res && res.success) {
        setDesktopActionMsg({
          text: res.message || "Desktop agent successfully reconnected and protection started!",
          type: "success",
        });
        if (res.status) {
          setDesktopAgent({ available: true, loading: false, status: res.status, error: "" });
        }
        await loadCoverage(true);
      } else {
        setDesktopActionMsg({
          text: res?.error || "Failed to start or connect to desktop agent.",
          type: "error",
        });
      }
    } catch (err) {
      setDesktopActionMsg({
        text: err.message || "Failed to connect to desktop agent.",
        type: "error",
      });
    } finally {
      setReconnecting(false);
    }
  };

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
  const devicesList = Array.isArray(data?.devices) ? data.devices : [];
  const activeDevices = devicesList.filter(
    (d) => (d.status || "").toUpperCase() !== "REVOKED"
  );
  const allEnforcing =
    activeDevices.length > 0 &&
    activeDevices.every((d) => d.enforcing && (!d.issues || d.issues.length === 0));

  return (
    <div className="card-stat rounded-2xl p-6 shadow-sm border border-white/[0.08] transition-all space-y-5">
      {/* Header with Title, Status Badges, and Action Toolbar */}
      <div className="flex flex-col xl:flex-row xl:items-center justify-between gap-4 border-b border-white/[0.08] pb-5">
        <div className="flex items-start sm:items-center gap-3">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl border border-white/20 bg-white/10 text-white shadow-sm shrink-0 mt-1 sm:mt-0">
            <Server size={22} />
          </div>
          <div>
            <div className="flex items-center gap-2 flex-wrap">
              <h3 className="text-base font-bold text-white tracking-tight">Active Device & Browser Coverage</h3>

              {/* Connected Devices Fleet Badge */}
              <span className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-[11px] font-bold ${allEnforcing ? "border-white/20 bg-white/10 text-white" : "border-amber-500/30 bg-amber-500/10 text-amber-300"
                }`}>
                {allEnforcing ? (
                  <>
                    <ShieldCheck size={12} className="text-white" />
                    <span>{activeDevices.length} Connected • All Enforcing</span>
                  </>
                ) : (
                  <>
                    <AlertTriangle size={12} className="text-amber-400" />
                    <span>{activeDevices.length} Connected • Issues Detected</span>
                  </>
                )}
              </span>

              {/* Desktop Daemon Status Pill */}
              <span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[10px] font-bold border ${desktopAgent.available && (desktopAgent.status?.enforcing || desktopAgent.status?.connected)
                ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300"
                : "border-neutral-700 bg-neutral-800 text-neutral-400"
                }`}>
                <span className={`h-1.5 w-1.5 rounded-full ${desktopAgent.available && (desktopAgent.status?.enforcing || desktopAgent.status?.connected)
                  ? "bg-emerald-400 animate-pulse"
                  : "bg-neutral-500"
                  }`} />
                <span>Desktop Daemon: {desktopAgent.available && (desktopAgent.status?.enforcing || desktopAgent.status?.connected) ? "Active & Enforcing" : "Stopped / Inactive"}</span>
              </span>

              {/* Machine-Wide DNS Shield Toggle Pill */}
              {desktopAgent.available && desktopAgent.status && (
                <button
                  type="button"
                  onClick={() => handleToggleDNS(desktopAgent.status.dnsRedirectionActive)}
                  disabled={dnsActionLoading}
                  className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[10px] font-bold border transition hover:opacity-80 disabled:opacity-50 ${desktopAgent.status.dnsRedirectionActive
                    ? "border-emerald-500/40 bg-emerald-500/20 text-emerald-300 font-bold"
                    : "border-white/10 bg-white/5 text-neutral-400 hover:text-white"
                    }`}
                  title="Click to toggle machine-wide loopback DNS shield"
                >
                  <Shield size={10} className={desktopAgent.status.dnsRedirectionActive ? "text-emerald-400" : "text-neutral-400"} />
                  <span>DNS Shield: {desktopAgent.status.dnsRedirectionActive ? "127.0.0.1 Active" : "Upstream"}</span>
                </button>
              )}
            </div>

            <p className="text-xs text-[#A1A1AA] mt-1">
              Live heartbeat telemetry across registered desktop agents, mobile clients, and connected browsers.
            </p>
          </div>
        </div>

        {/* Action Buttons Toolbar */}
        <div className="flex flex-wrap items-center gap-2 self-start xl:self-center shrink-0">
          <button
            type="button"
            onClick={handleReconnectDesktop}
            disabled={reconnecting}
            className="btn-premium inline-flex items-center gap-2 rounded-xl px-4 py-2 text-xs font-bold text-black bg-white hover:bg-neutral-200 transition disabled:opacity-50 shrink-0 shadow-md active:scale-95"
            title="Start or reconnect the background desktop enforcement daemon"
          >
            {reconnecting ? (
              <>
                <RefreshCw size={13} className="animate-spin text-black" />
                <span>Starting Daemon...</span>
              </>
            ) : (
              <>
                <Power size={13} />
                <span>Reconnect & Start Agent</span>
              </>
            )}
          </button>

          <a
            href="/downloads/OnuslySetup.exe"
            download="OnuslySetup.exe"
            className="btn-premium inline-flex items-center gap-1.5 rounded-xl border border-white/15 bg-white/5 px-3 py-2 text-xs font-semibold text-neutral-200 hover:text-white hover:bg-white/10 transition shadow-sm active:scale-95 shrink-0"
            title="Download ONUSLY for Windows installer (.exe)"
          >
            <Download size={13} />
            <span>Download Installer (.exe)</span>
          </a>

          <a
            href="/downloads/ONUSLY_Shield_Toggle.exe"
            download="ONUSLY_Shield_Toggle.exe"
            className="btn-premium inline-flex items-center gap-1.5 rounded-xl border border-white/15 bg-white/5 px-3 py-2 text-xs font-semibold text-neutral-200 hover:text-white hover:bg-white/10 transition shadow-sm active:scale-95 shrink-0"
            title="Download the 1-click Windows DNS switcher executable"
          >
            <Download size={13} />
            <span>1-Click Switcher (.exe)</span>
          </a>

          <button
            type="button"
            onClick={() => loadCoverage(true)}
            disabled={refreshing}
            className="btn-premium flex items-center justify-center rounded-xl border border-white/10 bg-white/5 p-2 text-neutral-300 hover:text-white hover:bg-white/10 transition disabled:opacity-50 shrink-0"
            title="Refresh device telemetry"
          >
            <RefreshCw size={14} className={refreshing ? "animate-spin" : ""} />
          </button>
        </div>
      </div>

      {/* Action Notification Messages */}
      {desktopActionMsg.text && (
        <div className={`p-2.5 rounded-xl text-xs font-medium flex items-center gap-2 border ${desktopActionMsg.type === "success"
          ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-200"
          : desktopActionMsg.type === "warning"
            ? "border-amber-500/30 bg-amber-500/10 text-amber-200"
            : "border-red-500/30 bg-red-500/10 text-red-200"
          }`}>
          {desktopActionMsg.type === "success" ? <CheckCircle2 size={13} /> : <AlertTriangle size={13} />}
          <span>{desktopActionMsg.text}</span>
        </div>
      )}

      {pairingMsg.text && (
        <div
          className={`p-2.5 rounded-lg text-xs font-semibold flex items-center gap-2 border ${pairingMsg.type === "success"
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
      ) : activeDevices.length === 0 ? (
        <div className="rounded-xl border border-dashed border-white/10 p-6 text-center text-xs text-neutral-400">
          <p className="font-semibold text-neutral-300">No active devices registered yet</p>
          <p className="mt-1 text-[11px] text-neutral-500">
            Install ONUSLY Desktop on Linux/Windows or sign into the browser extension to begin automated synchronized enforcement.
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
              {activeDevices.map((d) => {
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
                      <button
                        type="button"
                        onClick={() => {
                          setRevokeError("");
                          setDeviceToRevoke(d);
                        }}
                        className="inline-flex items-center gap-1 rounded-lg border border-red-500/30 bg-red-500/10 px-2.5 py-1 text-xs font-semibold text-red-400 hover:bg-red-500/20 hover:text-red-300 transition"
                        title="Revoke device authorization"
                      >
                        <Trash2 size={12} />
                        <span>Revoke</span>
                      </button>
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
