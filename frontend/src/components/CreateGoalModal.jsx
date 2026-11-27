import { useState, useEffect } from "react";
import { Lock, X, Loader2, Plus, Globe, Smartphone, Trash2, Laptop, AlertCircle, Check, AtSign } from "lucide-react";
import { createGoal, getFriendsList, lookupUser } from "../lib/api";
import { getStoredFriends } from "../lib/friendsChat";
import { APP_CATALOG, isProtectedProcess, normalizeDesktopAppTarget } from "../lib/blockedApps";
import { notifyExtensionSync } from "../lib/extensionSync";
import { playLockEngageSound } from "../lib/sound";

export default function CreateGoalModal({ isOpen, onClose, onGoalCreated, initialCustomTarget = null }) {
  if (!isOpen) return null;
  const modalKey = initialCustomTarget?.value ? `custom-${initialCustomTarget.value}` : "default";
  return (
    <CreateGoalModalInner
      key={modalKey}
      onClose={onClose}
      onGoalCreated={onGoalCreated}
      initialCustomTarget={initialCustomTarget}
    />
  );
}

function CreateGoalModalInner({ onClose, onGoalCreated, initialCustomTarget = null }) {
  const [taskDescription, setTaskDescription] = useState("");
  const [taskDetails, setTaskDetails] = useState("");
  const [selectedCatalogIds, setSelectedCatalogIds] = useState([]);
  const [customTargets, setCustomTargets] = useState(() =>
    initialCustomTarget && initialCustomTarget.value ? [initialCustomTarget] : []
  );
  const [customInput, setCustomInput] = useState("");
  const [customType, setCustomType] = useState("desktop"); // "desktop", "domain", "app"
  const [customTargetError, setCustomTargetError] = useState("");
  const [showCustomInput, setShowCustomInput] = useState(false);
  const [approvalType, setApprovalType] = useState("friend");
  const [approverEmail, setApproverEmail] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState("");
  const [partnerSuggestions, setPartnerSuggestions] = useState(() =>
    getStoredFriends().map((f) => ({
      id: f.id,
      name: f.name,
      handle: (f.handle || "").replace(/^@/, ""),
      email: f.email || "",
    }))
  );
  const [verifyingPartner, setVerifyingPartner] = useState(false);
  const [verifiedPartnerInfo, setVerifiedPartnerInfo] = useState(null);
  const [partnerLookupErr, setPartnerLookupErr] = useState("");

  useEffect(() => {
    let active = true;
    const localFriends = getStoredFriends().map((f) => ({
      id: f.id,
      name: f.name,
      handle: (f.handle || "").replace(/^@/, ""),
      email: f.email || "",
    }));

    getFriendsList()
      .then((serverList) => {
        if (!active || !Array.isArray(serverList)) return;
        const map = new Map();
        serverList.forEach((f) => {
          const key = (f.friendHandle || f.friendEmail || f.friendId || "").toLowerCase();
          if (key) {
            map.set(key, {
              id: f.friendId,
              name: f.friendName || f.friendHandle || f.friendEmail,
              handle: (f.friendHandle || "").replace(/^@/, ""),
              email: f.friendEmail || "",
            });
          }
        });
        localFriends.forEach((lf) => {
          const key = (lf.handle || lf.email || lf.id || "").toLowerCase();
          if (key && !map.has(key)) map.set(key, lf);
        });
        setPartnerSuggestions(Array.from(map.values()));
      })
      .catch(() => {});
    return () => { active = false; };
  }, []);

  const handleVerifyPartnerHandle = async () => {
    const raw = approverEmail.trim();
    if (!raw || raw.includes("@") && !raw.startsWith("@")) return;
    const clean = raw.replace(/^@/, "").toLowerCase();
    setVerifyingPartner(true);
    setPartnerLookupErr("");
    try {
      const profile = await lookupUser(clean);
      if (profile) {
        setVerifiedPartnerInfo(profile);
        setApproverEmail(`@${profile.handle}`);
      }
    } catch (err) {
      setVerifiedPartnerInfo(null);
      setPartnerLookupErr(err?.message || "Handle not found — try @handle or Gmail");
    } finally {
      setVerifyingPartner(false);
    }
  };

  const hasTargets = selectedCatalogIds.length > 0 || customTargets.length > 0;

  // Handle escape key
  useEffect(() => {
    const handleKeyDown = (e) => {
      if (e.key === "Escape" && !submitting) {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [submitting, onClose]);

  const toggleCatalogApp = (appId) => {
    setSelectedCatalogIds((prev) => {
      if (prev.includes(appId)) {
        return prev.filter((id) => id !== appId);
      } else {
        // Enforce strict isolation between full YouTube and YouTube Shorts:
        // Selecting "YouTube Shorts" automatically deselects full "YouTube".
        // Selecting full "YouTube" automatically deselects "YouTube Shorts".
        if (appId === "youtube_shorts") {
          return [...prev.filter((id) => id !== "youtube"), appId];
        } else if (appId === "youtube") {
          return [...prev.filter((id) => id !== "youtube_shorts"), appId];
        }
        return [...prev, appId];
      }
    });
  };

  const handleSelectAllCatalog = () => {
    // Keep youtube_shorts mutually exclusive with full youtube so Select All never triggers conflicts
    const compatibleIds = APP_CATALOG.filter((a) => a.id !== "youtube_shorts").map((a) => a.id);
    if (compatibleIds.every((id) => selectedCatalogIds.includes(id))) {
      setSelectedCatalogIds([]);
    } else {
      setSelectedCatalogIds(compatibleIds);
    }
  };

  const handleAddCustomTarget = () => {
    setCustomTargetError("");
    const trimmed = customInput.trim();
    if (!trimmed) return;

    if (customType === "desktop") {
      const clean = normalizeDesktopAppTarget(trimmed);
      if (isProtectedProcess(clean)) {
        setCustomTargetError(`"${clean}" is a protected operating system / ONUSLY component and cannot be restricted.`);
        return;
      }
      if (/[/\\&|;><$`*?~!(){}[\]'"\n\r\t]/.test(clean)) {
        setCustomTargetError("Invalid application identifier: shell metacharacters and directory paths are not allowed.");
        return;
      }
      if (!/^[a-zA-Z0-9][a-zA-Z0-9._-]*$/.test(clean)) {
        setCustomTargetError("Invalid application identifier format (must start with letter or number).");
        return;
      }
      if (customTargets.some((t) => t.value.toLowerCase() === clean.toLowerCase())) {
        setCustomTargetError(`"${clean}" is already in your custom targets.`);
        return;
      }
      setCustomTargets((prev) => [
        ...prev,
        {
          id: `custom-desk-${Date.now()}-${Math.random()}`,
          type: "desktop",
          value: clean,
        },
      ]);
      setCustomInput("");
      return;
    }

    if (customType === "domain") {
      const lower = trimmed.toLowerCase();
      // Auto-detect YouTube Shorts URLs/domains and route to granular Shorts target
      if (lower.includes("youtube.com/shorts") || lower.includes("youtu.be/shorts") || lower.includes("shorts.youtube.com")) {
        setSelectedCatalogIds((prev) => [...prev.filter((id) => id !== "youtube"), "youtube_shorts"]);
        setCustomInput("");
        return;
      }

      const cleanDomain = lower
        .replace(/^(?:https?:\/\/)?(?:www\.)?/i, "")
        .split("/")[0]
        .split("?")[0];
      if (!cleanDomain.includes(".")) {
        setCustomTargetError("Invalid domain format (e.g. youtube.com, reddit.com).");
        return;
      }
      if (cleanDomain === "youtube.com" || cleanDomain === "youtu.be") {
        // If explicitly adding full youtube domain, ensure youtube_shorts is unselected
        setSelectedCatalogIds((prev) => prev.filter((id) => id !== "youtube_shorts"));
      }
      if (customTargets.some((t) => t.value.toLowerCase() === cleanDomain)) {
        setCustomTargetError(`"${cleanDomain}" is already in your custom targets.`);
        return;
      }
      setCustomTargets((prev) => [
        ...prev,
        {
          id: `custom-dom-${Date.now()}-${Math.random()}`,
          type: "domain",
          value: cleanDomain,
        },
      ]);
      setCustomInput("");
      return;
    }

    if (customType === "app") {
      if (isProtectedProcess(trimmed)) {
        setCustomTargetError(`"${trimmed}" is a protected process and cannot be restricted.`);
        return;
      }
      if (!/^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$/.test(trimmed)) {
        setCustomTargetError("Android package must match format (e.g. com.example.app).");
        return;
      }
      if (customTargets.some((t) => t.value.toLowerCase() === trimmed.toLowerCase())) {
        setCustomTargetError(`"${trimmed}" is already in your custom targets.`);
        return;
      }
      setCustomTargets((prev) => [
        ...prev,
        {
          id: `custom-app-${Date.now()}-${Math.random()}`,
          type: "app",
          value: trimmed,
        },
      ]);
      setCustomInput("");
      return;
    }
  };

  const handleRemoveCustomTarget = (id) => {
    setCustomTargets((prev) => prev.filter((t) => t.id !== id));
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setFormError("");

    if (!taskDescription.trim()) {
      setFormError("Goal name / task description is required");
      return;
    }

    // Build lists of real package names and domains from catalog + custom
    const appsSet = new Set();
    const domainsSet = new Set();
    const applicationTargets = [];

    selectedCatalogIds.forEach((catId) => {
      const item = APP_CATALOG.find((c) => c.id === catId);
      if (item) {
        if (item.packageName) {
          appsSet.add(item.packageName);
          applicationTargets.push({ kind: "android_package", identifier: item.packageName });
        }
        if (Array.isArray(item.packageNames)) {
          item.packageNames.forEach((pkg) => {
            appsSet.add(pkg);
            const kind = pkg.includes(".") ? "android_package" : "desktop_app";
            applicationTargets.push({ kind, identifier: pkg });
          });
        }
        if (item.id && !item.packageName?.includes(item.id)) {
          appsSet.add(item.id);
          applicationTargets.push({ kind: "desktop_app", identifier: item.id });
        }
        (item.domains || []).forEach((d) => domainsSet.add(d));
      }
    });

    for (const ct of customTargets) {
      const rawVal = (ct.value || "").trim();
      if (!rawVal) continue;
      if (ct.type === "desktop") {
        const clean = normalizeDesktopAppTarget(rawVal);
        if (isProtectedProcess(clean)) {
          setFormError(`"${clean}" is a protected system/ONUSLY process and cannot be restricted.`);
          return;
        }
        appsSet.add(clean);
        applicationTargets.push({ kind: "desktop_app", identifier: clean });
      } else if (ct.type === "app") {
        if (isProtectedProcess(rawVal)) {
          setFormError(`"${rawVal}" is a protected process and cannot be restricted.`);
          return;
        }
        appsSet.add(rawVal);
        applicationTargets.push({ kind: "android_package", identifier: rawVal });
      } else {
        const cleanDom = rawVal.toLowerCase().replace(/^https?:\/\//, "").replace(/^www\./, "").split("/")[0];
        if (cleanDom) domainsSet.add(cleanDom);
      }
    }

    const isTargeted = appsSet.size > 0 || domainsSet.size > 0;
    const finalApprovalType = isTargeted ? "friend" : approvalType;
    const trimmedApprover = approverEmail.trim();

    if (isTargeted && !trimmedApprover) {
      setFormError("Accountability partner (@handle or email) is required when apps or websites are locked.");
      return;
    }

    if (finalApprovalType === "friend" && !trimmedApprover) {
      setFormError("Accountability partner (@handle or email) is required for friend verification.");
      return;
    }

    setSubmitting(true);
    try {
      const created = await createGoal({
        title: taskDescription.trim(),
        description: taskDetails.trim(),
        approvalType: finalApprovalType,
        approverEmail: trimmedApprover,
        blockedApps: Array.from(appsSet),
        blockedDomains: Array.from(domainsSet),
        applicationTargets,
      });

      if (onGoalCreated) {
        onGoalCreated(created);
      }
      playLockEngageSound();
      notifyExtensionSync();
      onClose();
    } catch (err) {
      console.error("Failed to create goal:", err);
      if (err.message && err.message.includes("accountability_required")) {
        setFormError("Accountability required: Goals with app or domain locks require an assigned accountability partner (friend approval).");
      } else {
        setFormError(err.message || "Failed to start accountability lock");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center modal-backdrop p-4"
      onClick={onClose}
    >
      <div
        className="w-full max-w-lg rounded-lg border border-[#2b3542] bg-[#12161c] p-6 sm:p-7 shadow-[0_25px_60px_rgba(0,0,0,0.95)] max-h-[90vh] overflow-y-auto animate-scale-in"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-[#26313d] pb-4">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded border border-[#2d3744] bg-[#182028] text-[#ff7843] shadow-sm">
              <Lock size={16} strokeWidth={2.5} />
            </div>
            <div>
              <h3 className="text-base font-bold text-white tracking-tight">
                New Accountability Lock
              </h3>
              <p className="text-xs text-[#8c9ba8]">
                Lock distracting apps and websites until approved.
              </p>
            </div>
          </div>

          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="rounded border border-[#26313d] bg-[#161c23] p-1.5 text-neutral-400 hover:text-white hover:border-[#384654] transition"
          >
            <X size={16} />
          </button>
        </div>

        {formError && (
          <div className="mt-4 rounded border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-300">
            {formError}
          </div>
        )}

        <form onSubmit={handleSubmit} className="mt-5 space-y-5">
          {/* 1. Goal / Task Name */}
          <div>
            <label className="block font-mono text-[10px] uppercase font-bold tracking-wider text-neutral-400 mb-1.5">
              1. Goal Name / Task <span className="text-[#ff7843]">*</span>
            </label>
            <input
              type="text"
              required
              autoFocus
              placeholder="e.g. Master DSA Graph Algorithms, Complete 5km Run..."
              value={taskDescription}
              onChange={(e) => setTaskDescription(e.target.value)}
              className="w-full rounded border border-[#26313d] bg-[#0c0f14] px-3.5 py-2.5 text-xs text-white placeholder-neutral-600 focus:border-[#ff7843] focus:outline-none transition"
            />
          </div>

          {/* 2. Goal Description / Notes */}
          <div>
            <label className="block font-mono text-[10px] uppercase font-bold tracking-wider text-neutral-400 mb-1.5">
              2. Description & Task Notes (Optional)
            </label>
            <textarea
              rows={3}
              placeholder="Describe what you plan to accomplish, key milestones, or notes..."
              value={taskDetails}
              onChange={(e) => setTaskDetails(e.target.value)}
              className="w-full rounded border border-[#26313d] bg-[#0c0f14] px-3.5 py-2 text-xs text-white placeholder-neutral-600 focus:border-[#ff7843] focus:outline-none transition resize-none"
            />
          </div>

          {/* 3. Catalog Picker */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="block font-mono text-[10px] uppercase font-bold tracking-wider text-neutral-400">
                3. Apps & Websites Catalog
              </label>
              <button
                type="button"
                onClick={handleSelectAllCatalog}
                className="font-mono text-[10px] uppercase tracking-wider text-[#ff7843] hover:underline"
              >
                {selectedCatalogIds.length === APP_CATALOG.length ? "Deselect All" : "Select All"}
              </button>
            </div>
            <p className="font-mono text-[10px] text-neutral-400 mb-3">
              Selected apps will be blocked on Android and desktop browser ({selectedCatalogIds.length} catalog items selected):
            </p>

            <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
              {APP_CATALOG.map((item) => {
                const isSelected = selectedCatalogIds.includes(item.id);
                return (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => toggleCatalogApp(item.id)}
                    className={`flex items-center justify-between rounded border p-2 text-xs font-mono transition-all ${
                      isSelected
                        ? "border-[#ff7843] bg-[#1c2430] text-white font-bold shadow-sm"
                        : "border-[#26313d] bg-[#0c0f14] text-neutral-400 hover:border-[#384654] hover:text-white"
                    }`}
                  >
                    <span>{item.name}</span>
                    <Lock size={10} className={isSelected ? "text-[#ff7843]" : "text-neutral-600"} />
                  </button>
                );
              })}
            </div>
          </div>

          {/* 4. Custom Target Option */}
          <div className="rounded border border-[#26313d] bg-[#0c0f14] p-3.5">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Plus size={13} className="text-[#ff7843]" />
                <span className="font-mono text-[10px] uppercase font-bold tracking-wider text-white">Custom Target</span>
              </div>
              <button
                type="button"
                onClick={() => setShowCustomInput((v) => !v)}
                className="font-mono text-[10px] uppercase tracking-wider text-neutral-400 hover:text-white transition underline"
              >
                {showCustomInput ? "Hide" : "+ Add Desktop App / Domain / Package"}
              </button>
            </div>

            {showCustomInput && (
              <div className="mt-3 space-y-3 border-t border-[#26313d] pt-3">
                <div className="flex flex-wrap items-center gap-2">
                  <button
                    type="button"
                    onClick={() => {
                      setCustomType("desktop");
                      setCustomTargetError("");
                    }}
                    className={`flex items-center gap-1.5 rounded border px-2.5 py-1 font-mono text-[10px] font-bold uppercase tracking-wider transition ${
                      customType === "desktop"
                        ? "border-[#ff7843] bg-[#1c2430] text-white shadow-sm"
                        : "border-[#26313d] bg-[#141920] text-neutral-400 hover:text-white"
                    }`}
                  >
                    <Laptop size={11} />
                    <span>Desktop App</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setCustomType("domain");
                      setCustomTargetError("");
                    }}
                    className={`flex items-center gap-1.5 rounded border px-2.5 py-1 font-mono text-[10px] font-bold uppercase tracking-wider transition ${
                      customType === "domain"
                        ? "border-[#ff7843] bg-[#1c2430] text-white shadow-sm"
                        : "border-[#26313d] bg-[#141920] text-neutral-400 hover:text-white"
                    }`}
                  >
                    <Globe size={11} />
                    <span>Website / Domain</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setCustomType("app");
                      setCustomTargetError("");
                    }}
                    className={`flex items-center gap-1.5 rounded border px-2.5 py-1 font-mono text-[10px] font-bold uppercase tracking-wider transition ${
                      customType === "app"
                        ? "border-[#ff7843] bg-[#1c2430] text-white shadow-sm"
                        : "border-[#26313d] bg-[#141920] text-neutral-400 hover:text-white"
                    }`}
                  >
                    <Smartphone size={11} />
                    <span>Android Package</span>
                  </button>
                </div>

                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    value={customInput}
                    onChange={(e) => {
                      setCustomInput(e.target.value);
                      if (customTargetError) setCustomTargetError("");
                    }}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        handleAddCustomTarget();
                      }
                    }}
                    placeholder={
                      customType === "desktop"
                        ? "e.g. discord, steam, spotify, or VLC.exe"
                        : customType === "domain"
                        ? "e.g. news.ycombinator.com"
                        : "e.g. com.snapchat.android"
                    }
                    className="flex-1 rounded border border-[#26313d] bg-[#080b0e] px-3 py-1.5 text-xs text-white placeholder-neutral-600 focus:border-[#ff7843] focus:outline-none transition"
                  />
                  <button
                    type="button"
                    onClick={handleAddCustomTarget}
                    className="rounded border border-[#384654] bg-[#1b232c] px-3 py-1.5 font-mono text-[10px] font-bold uppercase tracking-wider text-white hover:border-[#ff7843] hover:text-[#ff7843] transition"
                  >
                    Add
                  </button>
                </div>

                {/* Live Normalized Preview */}
                {customInput.trim() && (
                  <div className="text-[11px] px-1 font-mono">
                    {customType === "desktop" ? (
                      (() => {
                        const clean = normalizeDesktopAppTarget(customInput.trim());
                        if (isProtectedProcess(clean)) {
                          return (
                            <span className="inline-flex items-center gap-1 text-amber-400 font-mono text-[10px]">
                              <AlertCircle size={11} className="shrink-0" />
                              <span>"{clean}" is a protected system process and cannot be restricted.</span>
                            </span>
                          );
                        }
                        if (/[/\\&|;><$`*?~!(){}[\]'"\n\r\t]/.test(clean)) {
                          return (
                            <span className="inline-flex items-center gap-1 text-amber-400 font-mono text-[10px]">
                              <AlertCircle size={11} className="shrink-0" />
                              <span>Invalid characters: shell characters and paths are not allowed.</span>
                            </span>
                          );
                        }
                        return (
                          <span className="text-neutral-400">
                            Normalized Target: <span className="text-white font-bold">{clean}</span>
                          </span>
                        );
                      })()
                    ) : customType === "domain" ? (
                      <span className="text-neutral-400">
                        Normalized Domain:{" "}
                        <span className="text-white font-bold">
                          {customInput
                            .trim()
                            .toLowerCase()
                            .replace(/^(?:https?:\/\/)?(?:www\.)?/i, "")
                            .split("/")[0]
                            .split("?")[0]}
                        </span>
                      </span>
                    ) : (
                      <span className="text-neutral-400">
                        Target Package: <span className="text-white font-bold">{customInput.trim()}</span>
                      </span>
                    )}
                  </div>
                )}

                {customTargetError && (
                  <div className="text-[11px] text-amber-400 px-1 font-mono">
                    {customTargetError}
                  </div>
                )}

                {/* Tier 1 Detection & Response Disclosure */}
                <div className="rounded border border-[#26313d] bg-[#080b0e] p-3 font-mono text-[10px] text-neutral-400 flex items-start gap-2.5">
                  <AlertCircle size={14} className="text-neutral-500 shrink-0 mt-0.5" />
                  <div className="space-y-1">
                    <div>
                      <span className="font-semibold text-white">Tier 1 — Detection & Response:</span> Restricted desktop applications may briefly launch before ONUSLY detects and closes them.
                    </div>
                    <div className="text-[10px] text-neutral-500">
                      Immunity Notice: Core operating system binaries (e.g. Explorer, Task Manager, Command Prompt, PowerShell) and ONUSLY agent components are protected and cannot be restricted.
                    </div>
                  </div>
                </div>
              </div>
            )}

            {customTargets.length > 0 && (
              <div className="mt-3 flex flex-wrap gap-1.5 border-t border-[#26313d] pt-3">
                {customTargets.map((ct) => (
                  <span
                    key={ct.id}
                    className="inline-flex items-center gap-1.5 rounded border border-[#2b3542] bg-[#161c23] px-2 py-0.5 font-mono text-[10px] text-neutral-300"
                  >
                    {ct.type === "desktop" ? (
                      <Laptop size={10} className="text-neutral-400" />
                    ) : ct.type === "app" ? (
                      <Smartphone size={10} className="text-neutral-400" />
                    ) : (
                      <Globe size={10} className="text-neutral-400" />
                    )}
                    <span className="text-[9px] uppercase font-bold text-neutral-500 tracking-wider">
                      {ct.type === "desktop" ? "Desktop" : ct.type === "app" ? "Android" : "Web"}
                    </span>
                    <span>{ct.value}</span>
                    <button
                      type="button"
                      onClick={() => handleRemoveCustomTarget(ct.id)}
                      className="text-neutral-500 hover:text-white transition ml-0.5"
                    >
                      <Trash2 size={10} />
                    </button>
                  </span>
                ))}
              </div>
            )}
          </div>

          {/* 5. Accountability Verification */}
          <div className="rounded border border-[#26313d] bg-[#0c0f14] p-4 space-y-3">
            <div className="flex items-center justify-between">
              <label className="block font-mono text-[10px] uppercase font-bold tracking-wider text-neutral-400">
                5. Accountability Verification
              </label>
              {hasTargets ? (
                <span className="rounded border border-[#ff7843]/40 bg-[#ff7843]/15 px-2 py-0.5 font-mono text-[10px] font-bold uppercase tracking-wider text-[#ff9769]">
                  Locked: Friend Only
                </span>
              ) : (
                <span className="font-mono text-[10px] text-neutral-400">
                  Choose verification mode
                </span>
              )}
            </div>

            {hasTargets ? (
              <p className="text-xs text-neutral-400 leading-relaxed">
                Goals with distraction locks require friend verification. Only an assigned accountability partner can review your proof and unlock your targets.
              </p>
            ) : (
              <div className="grid grid-cols-2 gap-2">
                <button
                  type="button"
                  onClick={() => setApprovalType("friend")}
                  className={`rounded border p-2 text-xs font-mono font-bold uppercase tracking-wider transition ${
                    approvalType === "friend"
                      ? "border-[#ff7843] bg-[#1c2430] text-white shadow-sm"
                      : "border-[#26313d] bg-[#141920] text-neutral-400 hover:text-white"
                  }`}
                >
                  Friend
                </button>
                <button
                  type="button"
                  onClick={() => setApprovalType("none")}
                  className={`rounded border p-2 text-xs font-mono font-bold uppercase tracking-wider transition ${
                    approvalType === "none"
                      ? "border-[#ff7843] bg-[#1c2430] text-white shadow-sm"
                      : "border-[#26313d] bg-[#141920] text-neutral-400 hover:text-white"
                  }`}
                >
                  None
                </button>
              </div>
            )}

            {(hasTargets || approvalType === "friend") && (
              <div className="space-y-2 pt-1">
                <label className="block font-mono text-[10px] uppercase font-bold tracking-wider text-neutral-400">
                  Accountability Partner (@handle or email) <span className="text-[#ff7843]">*</span>
                </label>

                {partnerSuggestions.length > 0 && (
                  <div className="flex flex-wrap items-center gap-1.5">
                    {partnerSuggestions.slice(0, 6).map((p) => {
                      const tag = p.handle ? `@${p.handle}` : p.email;
                      const isSelected = approverEmail.trim().toLowerCase() === tag.toLowerCase();
                      return (
                        <button
                          key={p.id || tag}
                          type="button"
                          onClick={() => {
                            setApproverEmail(tag);
                            setVerifiedPartnerInfo(p.handle ? p : null);
                            setPartnerLookupErr("");
                          }}
                          className={`inline-flex items-center gap-1 rounded border px-2 py-1 font-mono text-[10px] transition ${
                            isSelected
                              ? "border-[#ff7843] bg-[#1c2430] text-white font-bold"
                              : "border-[#26313d] bg-[#080b0e] text-neutral-400 hover:border-[#384654] hover:text-white"
                          }`}
                        >
                          <AtSign size={10} className="text-[#ff7843]" />
                          <span>{p.handle || p.name}</span>
                        </button>
                      );
                    })}
                  </div>
                )}

                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    required={hasTargets || approvalType === "friend"}
                    placeholder="e.g. @partner, partner_handle, or friend@gmail.com"
                    value={approverEmail}
                    onChange={(e) => {
                      setApproverEmail(e.target.value);
                      setVerifiedPartnerInfo(null);
                      setPartnerLookupErr("");
                    }}
                    className="flex-1 rounded border border-[#26313d] bg-[#080b0e] px-3.5 py-2 text-xs text-white placeholder-neutral-600 focus:border-[#ff7843] focus:outline-none transition"
                  />
                  {approverEmail.trim() && (!approverEmail.includes("@") || approverEmail.trim().startsWith("@")) && (
                    <button
                      type="button"
                      onClick={handleVerifyPartnerHandle}
                      disabled={verifyingPartner}
                      className="rounded border border-[#384654] bg-[#1b232c] px-3 py-2 font-mono text-[10px] font-bold uppercase tracking-wider text-white hover:border-[#ff7843] hover:text-[#ff7843] disabled:opacity-50 transition shrink-0"
                    >
                      {verifyingPartner ? <Loader2 size={12} className="animate-spin" /> : "Verify"}
                    </button>
                  )}
                </div>

                {verifiedPartnerInfo && (
                  <p className="inline-flex items-center gap-1.5 font-mono text-[10px] text-emerald-400">
                    <Check size={11} />
                    <span>Verified partner: {verifiedPartnerInfo.name} (@{verifiedPartnerInfo.handle})</span>
                  </p>
                )}
                {partnerLookupErr && (
                  <p className="font-mono text-[10px] text-amber-400">
                    {partnerLookupErr}
                  </p>
                )}
                <p className="font-mono text-[10px] text-neutral-500">
                  Your partner must verify your proof submission to lift distraction locks.
                </p>
              </div>
            )}
          </div>

          {/* Action buttons */}
          <div className="pt-2 flex items-center gap-3">
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="flex-1 rounded border border-[#26313d] bg-[#161c23] py-2.5 font-mono text-xs font-bold uppercase tracking-wider text-neutral-300 hover:text-white hover:border-[#384654] transition"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={submitting || !taskDescription.trim()}
              className="flex-1 flex items-center justify-center gap-2 rounded border border-[#ff7843] bg-[#ff7843] py-2.5 font-mono text-xs font-bold uppercase tracking-wider text-black shadow-lg hover:bg-[#ff8f5e] transition disabled:opacity-40 active:scale-95"
            >
              {submitting ? (
                <>
                  <Loader2 size={14} className="animate-spin text-black" />
                  <span>Activating Lock...</span>
                </>
              ) : (
                <>
                  <Lock size={13} strokeWidth={2.5} />
                  <span>Start Accountability Lock</span>
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
