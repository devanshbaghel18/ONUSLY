import { useState, useEffect, useCallback } from "react";
import {
  Laptop,
  Smartphone,
  Globe,
  ShieldCheck,
  AlertTriangle,
  RefreshCw,
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
      <span className="inline-flex items-center gap-1 rounded border border-red-500/30 bg-red-500/10 px-2 py-0.5 font-mono text-[9px] font-bold uppercase tracking-wider text-red-400">
        <Ban size={10} />
        Revoked
      </span>
    );
  }
  if (status === "AUTHENTICATION_REQUIRED" || status === "AUTH_REQUIRED" || health === "AUTHENTICATION_REQUIRED") {
    return (
      <span className="inline-flex items-center gap-1 rounded border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 font-mono text-[9px] font-bold uppercase tracking-wider text-amber-300">
        <AlertTriangle size={10} />
        Auth Required
      </span>
    );
  }
  if (health === "DNS_UNAVAILABLE") {
    return (
      <div>
        <span className="inline-flex items-center gap-1 rounded border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 font-mono text-[9px] font-bold uppercase tracking-wider text-amber-300">
          <AlertTriangle size={10} />
          DNS Unavailable
        </span>
        <div className="font-mono text-[9px] text-amber-400/80 mt-0.5">Loopback DNS resolver stopped</div>
      </div>
    );
  }
  if (health === "APP_ENFORCEMENT_DEGRADED") {
    return (
      <div>
        <span className="inline-flex items-center gap-1 rounded border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 font-mono text-[9px] font-bold uppercase tracking-wider text-amber-300">
          <AlertTriangle size={10} />
          App Monitor Degraded
        </span>
      </div>
    );
  }
  if (!d.enforcing || (d.issues && d.issues.length > 0) || health === "DEGRADED" || health === "POLICY_MISSING" || health === "POLICY_STALE") {
    return (
      <div>
        <span className="inline-flex items-center gap-1 rounded border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 font-mono text-[9px] font-bold uppercase tracking-wider text-amber-300">
          <AlertTriangle size={10} />
          Degraded
        </span>
        {d.issues && d.issues.length > 0 && (
          <div className="font-mono text-[9px] text-amber-400/80 mt-0.5">
            {d.issues.join(", ").replace(/_/g, " ")}
          </div>
        )}
      </div>
    );
  }
  const isRecent = d.lastHeartbeatAt && Date.now() - new Date(d.lastHeartbeatAt).getTime() < 5 * 60 * 1000;
  if (!isRecent) {
    return (
      <span className="inline-flex items-center gap-1.5 rounded border border-[#26313d] bg-[#161c23] px-2 py-0.5 font-mono text-[9px] font-bold uppercase tracking-wider text-neutral-400">
        <span className="h-1.5 w-1.5 rounded-full bg-neutral-500" />
        Offline
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1.5 rounded border border-[#10b981]/30 bg-[#10b981]/10 px-2 py-0.5 font-mono text-[9px] font-bold uppercase tracking-wider text-[#10b981]">
      <span className="h-1.5 w-1.5 rounded-full bg-[#10b981] animate-pulse" />
      Online
    </span>
  );
}

export default function DeviceCoverageSection() {
  const [data, setData] = useState({ userId: "", totalCount: 0, enforcing: false, devices: [] });
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [pairingMsg, setPairingMsg] = useState({ text: "", type: "" });
  const [deviceToRevoke, setDeviceToRevoke] = useState(null);
  const [revoking, setRevoking] = useState(false);
  const [revokeError, setRevokeError] = useState("");

  // Machine Desktop Agent state
  const [desktopAgent, setDesktopAgent] = useState({ available: false, loading: true, status: null, error: "" });
  const [dnsActionLoading, setDnsActionLoading] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const [desktopActionMsg, setDesktopActionMsg] = useState({ text: "", type: "" });
  const [manualPairCode, setManualPairCode] = useState("");
  const [manualPairing, setManualPairing] = useState(false);
  const [showPairInput, setShowPairInput] = useState(false);
  const [fetchedAtMs, setFetchedAtMs] = useState(() => Date.now());

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
        setFetchedAtMs(Date.now());
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
          setFetchedAtMs(Date.now());
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
      confirmDevicePairing(clean)
        .then(() => {
          setPairingMsg({ text: "Device paired successfully! Synchronizing...", type: "success" });
          loadCoverage(true);
        })
        .catch((err) => {
          setPairingMsg({ text: err.message || "Failed to pair device.", type: "error" });
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

  const handleReconnectDesktop = async () => {
    setReconnecting(true);
    setDesktopActionMsg({ text: "", type: "" });
    try {
      const res = await reconnectDesktopAgent();
      if (res && res.success) {
        setShowPairInput(false);
        setDesktopActionMsg({
          text: res.message || "Desktop agent successfully reconnected and protection started!",
          type: "success",
        });
        if (res.status) {
          setDesktopAgent({ available: true, loading: false, status: res.status, error: "" });
        }
        await loadCoverage(true);
      } else {
        if (res?.requiresSetup) {
          setShowPairInput(true);
        }
        setDesktopActionMsg({
          text: res?.error || "Failed to start or connect to desktop agent.",
          type: res?.requiresSetup ? "warning" : "error",
        });
      }
    } catch (err) {
      setShowPairInput(true);
      setDesktopActionMsg({
        text: err.message || "Failed to connect to desktop agent.",
        type: "error",
      });
    } finally {
      setReconnecting(false);
    }
  };

  const handleManualPairSubmit = async (e) => {
    e.preventDefault();
    const clean = manualPairCode.trim().toUpperCase();
    if (clean.length !== 6) {
      setPairingMsg({ text: "Enter the 6-character pairing code shown by OnuslySetup.exe or `onusly-agent pair`.", type: "error" });
      return;
    }
    setManualPairing(true);
    setPairingMsg({ text: "", type: "" });
    try {
      await confirmDevicePairing(clean);
      setManualPairCode("");
      setShowPairInput(false);
      setPairingMsg({ text: `Device pairing confirmed for code ${clean}! Synchronizing fleet...`, type: "success" });
      await loadCoverage(true);
    } catch (err) {
      setPairingMsg({ text: err.message || "Failed to confirm pairing code.", type: "error" });
    } finally {
      setManualPairing(false);
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
  const fleetDesktopOnline = activeDevices.some(
    (d) =>
      (d.clientType === "desktop_agent" || d.clientType === "desktop-agent") &&
      d.lastHeartbeatAt &&
      fetchedAtMs - new Date(d.lastHeartbeatAt).getTime() < 5 * 60 * 1000
  );
  const desktopDaemonActive =
    (desktopAgent.available && (desktopAgent.status?.enforcing || desktopAgent.status?.connected)) ||
    fleetDesktopOnline;
  const allEnforcing =
    activeDevices.length > 0 &&
    activeDevices.every((d) => d.enforcing && (!d.issues || d.issues.length === 0));

  return (
    <div className="rounded-lg border border-[#26313d] bg-[#12161c] p-6 shadow-sm transition-all space-y-5">
      {/* Header with Title, Status Badges, and Action Toolbar */}
      <div className="flex flex-col xl:flex-row xl:items-center justify-between gap-4 border-b border-[#26313d] pb-5">
        <div className="flex items-start sm:items-center gap-3">
          <div className="flex h-9 w-9 items-center justify-center rounded border border-[#2d3744] bg-[#182028] text-[#ff7843] shadow-sm shrink-0 mt-1 sm:mt-0">
            <Server size={18} />
          </div>
          <div>
            <div className="flex items-center gap-2 flex-wrap">
              <h3 className="text-base font-bold text-white tracking-tight">Active Device & Browser Coverage</h3>

              {/* Connected Devices Fleet Badge */}
              <span className={`inline-flex items-center gap-1.5 rounded border px-2 py-0.5 font-mono text-[9px] font-bold uppercase tracking-wider ${allEnforcing ? "border-[#10b981]/30 bg-[#10b981]/10 text-[#10b981]" : "border-amber-500/30 bg-amber-500/10 text-amber-300"
                }`}>
                {allEnforcing ? (
                  <>
                    <ShieldCheck size={11} className="text-[#10b981]" />
                    <span>{activeDevices.length} Connected • All Enforcing</span>
                  </>
                ) : (
                  <>
                    <AlertTriangle size={11} className="text-amber-400" />
                    <span>{activeDevices.length} Connected • Issues Detected</span>
                  </>
                )}
              </span>

              {/* Desktop Daemon Status Pill */}
              <span className={`inline-flex items-center gap-1.5 rounded border px-2 py-0.5 font-mono text-[9px] font-bold uppercase tracking-wider ${desktopDaemonActive
                ? "border-[#10b981]/30 bg-[#10b981]/10 text-[#10b981]"
                : "border-[#26313d] bg-[#161c23] text-neutral-400"
                }`}>
                <span className={`h-1.5 w-1.5 rounded-full ${desktopDaemonActive
                  ? "bg-[#10b981] animate-pulse"
                  : "bg-neutral-500"
                  }`} />
                <span>Desktop Daemon: {desktopDaemonActive ? "Active & Enforcing" : "Not Connected"}</span>
              </span>

              {/* Machine-Wide DNS Shield Toggle Pill */}
              {desktopAgent.available && desktopAgent.status && (
                <button
                  type="button"
                  onClick={() => handleToggleDNS(desktopAgent.status.dnsRedirectionActive)}
                  disabled={dnsActionLoading}
                  className={`inline-flex items-center gap-1.5 rounded border px-2 py-0.5 font-mono text-[9px] font-bold uppercase tracking-wider transition hover:opacity-80 disabled:opacity-50 ${desktopAgent.status.dnsRedirectionActive
                    ? "border-[#10b981]/40 bg-[#10b981]/20 text-[#10b981] font-bold"
                    : "border-[#26313d] bg-[#161c23] text-neutral-400 hover:text-white"
                    }`}
                  title="Click to toggle machine-wide loopback DNS shield"
                >
                  <Shield size={10} className={desktopAgent.status.dnsRedirectionActive ? "text-[#10b981]" : "text-neutral-400"} />
                  <span>DNS Shield: {desktopAgent.status.dnsRedirectionActive ? "127.0.0.1 Active" : "Upstream"}</span>
                </button>
              )}
            </div>

            <p className="text-xs text-[#8c9ba8] mt-1">
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
            className="inline-flex items-center gap-2 rounded border border-[#ff7843] bg-[#ff7843] px-3.5 py-1.5 font-mono text-[11px] font-bold uppercase tracking-wider text-black hover:bg-[#ff8f5e] transition disabled:opacity-50 shrink-0 shadow-md active:scale-95"
            title="Start or reconnect the background desktop enforcement daemon"
          >
            {reconnecting ? (
              <>
                <RefreshCw size={12} className="animate-spin text-black" />
                <span>Starting Daemon...</span>
              </>
            ) : (
              <>
                <Power size={12} />
                <span>Reconnect & Start Agent</span>
              </>
            )}
          </button>

          <a
            href="/downloads/OnuslySetup.exe"
            download="OnuslySetup.exe"
            className="inline-flex items-center gap-1.5 rounded border border-[#26313d] bg-[#161c23] px-3 py-1.5 font-mono text-[11px] font-bold uppercase tracking-wider text-neutral-300 hover:text-white hover:border-[#384654] transition shadow-sm active:scale-95 shrink-0"
            title="Download ONUSLY for Windows installer (.exe)"
          >
            <Download size={12} />
            <span>Download Installer (.exe)</span>
          </a>

          <a
            href="/downloads/ONUSLY_Shield_Toggle.exe"
            download="ONUSLY_Shield_Toggle.exe"
            className="inline-flex items-center gap-1.5 rounded border border-[#26313d] bg-[#161c23] px-3 py-1.5 font-mono text-[11px] font-bold uppercase tracking-wider text-neutral-300 hover:text-white hover:border-[#384654] transition shadow-sm active:scale-95 shrink-0"
            title="Download the 1-click Windows DNS switcher executable"
          >
            <Download size={12} />
            <span>1-Click Switcher (.exe)</span>
          </a>

          <a
            href="/downloads/onusly.apk"
            download="onusly.apk"
            className="inline-flex items-center gap-1.5 rounded border border-[#26313d] bg-[#161c23] px-3 py-1.5 font-mono text-[11px] font-bold uppercase tracking-wider text-neutral-300 hover:text-white hover:border-[#384654] transition shadow-sm active:scale-95 shrink-0"
            title="Download ONUSLY for Android (.apk)"
          >
            <Download size={12} />
            <span>Android (.apk)</span>
          </a>

          <button
            type="button"
            onClick={() => setShowPairInput((prev) => !prev)}
            className="inline-flex items-center gap-1.5 rounded border border-[#26313d] bg-[#161c23] px-3 py-1.5 font-mono text-[11px] font-bold uppercase tracking-wider text-neutral-300 hover:text-white hover:border-[#384654] transition shadow-sm active:scale-95 shrink-0"
            title="Pair a desktop agent using its 6-character code"
          >
            <span>Pair Code</span>
          </button>

          <button
            type="button"
            onClick={() => loadCoverage(true)}
            disabled={refreshing}
            className="flex items-center justify-center rounded border border-[#26313d] bg-[#161c23] p-1.5 text-neutral-300 hover:text-white hover:border-[#384654] transition disabled:opacity-50 shrink-0"
            title="Refresh device telemetry"
          >
            <RefreshCw size={13} className={refreshing ? "animate-spin" : ""} />
          </button>
        </div>
      </div>

      {/* Action Notification Messages */}
      {desktopActionMsg.text && (
        <div className={`p-2.5 rounded border text-xs font-mono font-medium flex items-center gap-2 ${desktopActionMsg.type === "success"
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
          className={`p-2.5 rounded border text-xs font-mono font-semibold flex items-center gap-2 ${pairingMsg.type === "success"
            ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300"
            : "border-red-500/20 bg-red-500/10 text-red-300"
            }`}
        >
          {pairingMsg.type === "success" ? <CheckCircle2 size={13} /> : <AlertTriangle size={13} />}
          <span>{pairingMsg.text}</span>
        </div>
      )}

      {showPairInput && (
        <form
          onSubmit={handleManualPairSubmit}
          className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2 rounded border border-[#26313d] bg-[#0c0f14] p-3"
        >
          <span className="font-mono text-[11px] text-neutral-300 shrink-0">
            Desktop 6-Char Pairing Code:
          </span>
          <input
            type="text"
            value={manualPairCode}
            onChange={(e) => setManualPairCode(e.target.value.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 6))}
            placeholder="e.g. AB12CD"
            maxLength={6}
            className="w-full sm:w-40 rounded border border-[#2d3744] bg-[#161c23] px-3 py-1.5 font-mono text-xs font-bold uppercase tracking-widest text-white placeholder:text-neutral-500 focus:border-[#ff7843] focus:outline-none"
          />
          <button
            type="submit"
            disabled={manualPairing || manualPairCode.trim().length !== 6}
            className="inline-flex items-center justify-center rounded bg-[#ff7843] px-3.5 py-1.5 font-mono text-[11px] font-bold uppercase tracking-wider text-black hover:bg-[#ff8f5e] transition disabled:opacity-50"
          >
            {manualPairing ? "Pairing..." : "Confirm Pair"}
          </button>
        </form>
      )}

      {/* Devices List Table/Cards */}
      {loading ? (
        <div className="py-8 text-center text-xs font-mono text-neutral-500">
          <RefreshCw size={18} className="animate-spin mx-auto mb-2 opacity-50" />
          Loading connected devices...
        </div>
      ) : activeDevices.length === 0 ? (
        <div className="rounded border border-dashed border-[#26313d] bg-[#0c0f14] p-6 text-center text-xs text-neutral-400 font-mono">
          <p className="font-semibold text-neutral-300 uppercase tracking-wider">No active devices registered yet</p>
          <p className="mt-1 text-[11px] text-neutral-500">
            Install ONUSLY Desktop on Linux/Windows or sign into the browser extension to begin automated synchronized enforcement.
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead>
              <tr className="border-b border-[#26313d] font-mono text-[10px] uppercase font-bold tracking-wider text-neutral-400">
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
            <tbody className="divide-y divide-[#1e2630]">
              {activeDevices.map((d) => {
                return (
                  <tr key={d.deviceId} className="hover:bg-[#161c23] transition">
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
                    <td className="py-3 text-neutral-400 font-mono text-[11px]">{formatRelativeTime(d.lastHeartbeatAt)}</td>
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
                        className="inline-flex items-center gap-1 rounded border border-red-500/30 bg-red-500/10 px-2 py-0.5 font-mono text-[10px] font-bold uppercase tracking-wider text-red-400 hover:bg-red-500/20 hover:text-red-300 transition"
                        title="Revoke device authorization"
                      >
                        <Trash2 size={11} />
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
        <div className="fixed inset-0 z-50 flex items-center justify-center modal-backdrop p-4">
          <div className="w-full max-w-md rounded-lg border border-[#2b3542] bg-[#12161c] p-6 shadow-2xl space-y-4 animate-scale-in">
            <div className="flex items-center justify-between border-b border-[#26313d] pb-3">
              <h3 className="text-base font-bold text-white flex items-center gap-2">
                <AlertTriangle size={16} className="text-red-400" />
                Revoke Device Authorization
              </h3>
              <button
                type="button"
                onClick={() => setDeviceToRevoke(null)}
                disabled={revoking}
                className="rounded border border-[#26313d] bg-[#161c23] p-1 text-neutral-400 hover:text-white hover:border-[#384654] transition"
              >
                <X size={16} />
              </button>
            </div>

            <p className="text-xs text-neutral-300 leading-relaxed">
              Are you sure you want to revoke authorization for this device?
            </p>

            <div className="rounded border border-[#26313d] bg-[#0c0f14] p-3.5 space-y-2 font-mono text-xs">
              <div className="flex justify-between">
                <span className="text-neutral-400">Device ID:</span>
                <span className="text-white">{deviceToRevoke.deviceId}</span>
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

            <div className="rounded border border-red-500/20 bg-red-500/10 p-3 font-mono text-xs text-red-300 space-y-1">
              <p className="font-bold uppercase tracking-wider">Security Notice:</p>
              <p className="text-[10px] leading-relaxed">
                Revoking this device will permanently invalidate its credentials, disable automated enforcement, and require re-pairing to reconnect.
              </p>
            </div>

            {revokeError && (
              <div className="rounded border border-red-500/30 bg-red-500/20 p-2.5 font-mono text-xs text-red-200">
                {revokeError}
              </div>
            )}

            <div className="flex items-center justify-end gap-3 pt-2">
              <button
                type="button"
                onClick={() => setDeviceToRevoke(null)}
                disabled={revoking}
                className="rounded border border-[#26313d] bg-[#161c23] px-4 py-2 font-mono text-xs font-bold uppercase tracking-wider text-neutral-300 hover:border-[#384654] hover:text-white transition disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleConfirmRevoke}
                disabled={revoking}
                className="inline-flex items-center gap-1.5 rounded border border-red-500 bg-red-600 px-4 py-2 font-mono text-xs font-bold uppercase tracking-wider text-white hover:bg-red-500 transition disabled:opacity-50"
              >
                {revoking ? <RefreshCw size={12} className="animate-spin text-white" /> : <Ban size={12} />}
                <span>{revoking ? "Revoking..." : "Confirm Revocation"}</span>
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
