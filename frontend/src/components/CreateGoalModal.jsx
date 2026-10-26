import { useState, useEffect } from "react";
import { Lock, X, Loader2, Plus, Globe, Smartphone, Trash2 } from "lucide-react";
import { createGoal } from "../lib/api";
import { APP_CATALOG } from "../lib/blockedApps";
import { notifyExtensionSync } from "../lib/extensionSync";

export default function CreateGoalModal({ isOpen, onClose, onGoalCreated }) {
  const [taskDescription, setTaskDescription] = useState("");
  const [taskDetails, setTaskDetails] = useState("");
  const [selectedCatalogIds, setSelectedCatalogIds] = useState(["youtube", "instagram", "twitter"]);
  const [customTargets, setCustomTargets] = useState([]);
  const [customInput, setCustomInput] = useState("");
  const [customType, setCustomType] = useState("app"); // "app" or "domain"
  const [showCustomInput, setShowCustomInput] = useState(false);
  const [approvalType, setApprovalType] = useState("friend");
  const [approverEmail, setApproverEmail] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState("");

  const hasTargets = selectedCatalogIds.length > 0 || customTargets.length > 0;

  useEffect(() => {
    if (isOpen) {
      const timer = setTimeout(() => {
        setTaskDescription("");
        setTaskDetails("");
        setSelectedCatalogIds(["youtube", "instagram", "twitter"]);
        setCustomTargets([]);
        setCustomInput("");
        setShowCustomInput(false);
        setApprovalType("friend");
        setApproverEmail("");
        setFormError("");
      }, 0);
      return () => clearTimeout(timer);
    }
  }, [isOpen]);

  // Handle escape key
  useEffect(() => {
    const handleKeyDown = (e) => {
      if (e.key === "Escape" && isOpen && !submitting) {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, submitting, onClose]);

  if (!isOpen) return null;

  const toggleCatalogApp = (appId) => {
    setSelectedCatalogIds((prev) =>
      prev.includes(appId)
        ? prev.filter((id) => id !== appId)
        : [...prev, appId]
    );
  };

  const handleSelectAllCatalog = () => {
    if (selectedCatalogIds.length === APP_CATALOG.length) {
      setSelectedCatalogIds([]);
    } else {
      setSelectedCatalogIds(APP_CATALOG.map((a) => a.id));
    }
  };

  const handleAddCustomTarget = () => {
    const trimmed = customInput.trim();
    if (!trimmed) return;

    const lower = trimmed.toLowerCase();
    const cleanDomain = lower
      .replace(/^(?:https?:\/\/)?(?:www\.)?/i, "")
      .split("/")[0]
      .split("?")[0];

    // Check if entered target matches any entry in APP_CATALOG
    const catalogMatch = APP_CATALOG.find((item) => {
      const matchPkg =
        item.packageName?.toLowerCase() === lower ||
        (Array.isArray(item.packageNames) &&
          item.packageNames.some((p) => p.toLowerCase() === lower));
      const matchDom =
        Array.isArray(item.domains) &&
        item.domains.some((d) => {
          const dClean = d.toLowerCase().replace(/^(?:www\.)?/i, "");
          return dClean === cleanDomain || dClean === lower || d.toLowerCase() === lower;
        });
      return matchPkg || matchDom;
    });

    const newTargets = [];
    if (!customTargets.some((t) => t.value.toLowerCase() === trimmed.toLowerCase())) {
      newTargets.push({
        id: `custom-${Date.now()}-${Math.random()}`,
        type: customType,
        value: trimmed,
      });
    }

    // If user adds a custom website domain in APP_CATALOG (e.g. instagram.com),
    // also add the mapped package name (com.instagram.android) to apps so the phone app is also blocked.
    // If user adds a custom app package in APP_CATALOG, also add the mapped domains.
    if (catalogMatch) {
      if (customType === "domain" && catalogMatch.packageName) {
        if (
          !customTargets.some(
            (t) => t.value.toLowerCase() === catalogMatch.packageName.toLowerCase()
          ) &&
          !newTargets.some(
            (t) => t.value.toLowerCase() === catalogMatch.packageName.toLowerCase()
          )
        ) {
          newTargets.push({
            id: `custom-pkg-${Date.now()}-${Math.random()}`,
            type: "app",
            value: catalogMatch.packageName,
          });
        }
      } else if (customType === "app" && Array.isArray(catalogMatch.domains)) {
        catalogMatch.domains.forEach((d) => {
          if (
            !customTargets.some((t) => t.value.toLowerCase() === d.toLowerCase()) &&
            !newTargets.some((t) => t.value.toLowerCase() === d.toLowerCase())
          ) {
            newTargets.push({
              id: `custom-dom-${Date.now()}-${Math.random()}`,
              type: "domain",
              value: d,
            });
          }
        });
      }
    }

    if (newTargets.length > 0) {
      setCustomTargets((prev) => [...prev, ...newTargets]);
    }
    setCustomInput("");
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

    selectedCatalogIds.forEach((catId) => {
      const item = APP_CATALOG.find((c) => c.id === catId);
      if (item) {
        if (item.packageName) appsSet.add(item.packageName);
        if (Array.isArray(item.packageNames)) {
          item.packageNames.forEach((pkg) => appsSet.add(pkg));
        }
        (item.domains || []).forEach((d) => domainsSet.add(d));
      }
    });

    customTargets.forEach((ct) => {
      const trimmedVal = ct.value.trim();
      const lower = trimmedVal.toLowerCase();
      const cleanDomain = lower
        .replace(/^(?:https?:\/\/)?(?:www\.)?/i, "")
        .split("/")[0]
        .split("?")[0];

      const catalogMatch = APP_CATALOG.find((item) => {
        const matchPkg =
          item.packageName?.toLowerCase() === lower ||
          (Array.isArray(item.packageNames) &&
            item.packageNames.some((p) => p.toLowerCase() === lower));
        const matchDom =
          Array.isArray(item.domains) &&
          item.domains.some((d) => {
            const dClean = d.toLowerCase().replace(/^(?:www\.)?/i, "");
            return dClean === cleanDomain || dClean === lower || d.toLowerCase() === lower;
          });
        return matchPkg || matchDom;
      });

      if (ct.type === "app") {
        appsSet.add(trimmedVal);
        if (catalogMatch && Array.isArray(catalogMatch.domains)) {
          catalogMatch.domains.forEach((d) => domainsSet.add(d));
        }
      } else {
        domainsSet.add(trimmedVal);
        if (catalogMatch) {
          if (catalogMatch.packageName) appsSet.add(catalogMatch.packageName);
          if (Array.isArray(catalogMatch.packageNames)) {
            catalogMatch.packageNames.forEach((pkg) => appsSet.add(pkg));
          }
        }
      }
    });

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
      });

      if (onGoalCreated) {
        onGoalCreated(created);
      }
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
        className="w-full max-w-lg rounded-2xl border border-white/10 bg-[#141418]/95 backdrop-blur-2xl p-6 sm:p-7 shadow-[0_25px_60px_rgba(0,0,0,0.9)] max-h-[90vh] overflow-y-auto animate-scale-in"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-white/[0.08] pb-4">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-white text-black shadow-lg">
              <Lock size={18} strokeWidth={2.5} />
            </div>
            <div>
              <h3 className="text-base font-bold text-white">
                New Accountability Lock
              </h3>
              <p className="text-xs text-[#A1A1AA]">
                Lock distracting apps and websites until approved.
              </p>
            </div>
          </div>

          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="rounded-lg p-1.5 text-neutral-400 hover:text-white hover:bg-white/10 transition"
          >
            <X size={18} />
          </button>
        </div>

        {formError && (
          <div className="mt-4 rounded-xl border border-white/10 bg-white/[0.04] p-3 text-xs text-neutral-300">
            {formError}
          </div>
        )}

        <form onSubmit={handleSubmit} className="mt-5 space-y-5">
          {/* 1. Goal / Task Name */}
          <div>
            <label className="block text-xs font-bold uppercase tracking-wider text-white mb-1.5">
              1. Goal Name / Task <span className="text-neutral-500">*</span>
            </label>
            <input
              type="text"
              required
              autoFocus
              placeholder="e.g. Master DSA Graph Algorithms, Complete 5km Run..."
              value={taskDescription}
              onChange={(e) => setTaskDescription(e.target.value)}
              className="w-full rounded-xl border border-white/10 bg-[#0E0E12] px-4 py-3 text-sm text-white placeholder-neutral-500 focus:border-white/40 focus:ring-2 focus:ring-white/10 focus:outline-none transition"
            />
          </div>

          {/* 2. Goal Description / Notes */}
          <div>
            <label className="block text-xs font-bold uppercase tracking-wider text-white mb-1.5">
              2. Description & Task Notes (Optional)
            </label>
            <textarea
              rows={3}
              placeholder="Describe what you plan to accomplish, key milestones, or notes..."
              value={taskDetails}
              onChange={(e) => setTaskDetails(e.target.value)}
              className="w-full rounded-xl border border-white/10 bg-[#0E0E12] px-4 py-2.5 text-xs text-white placeholder-neutral-500 focus:border-white/40 focus:ring-2 focus:ring-white/10 focus:outline-none transition resize-none"
            />
          </div>

          {/* 3. Catalog Picker */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="block text-xs font-bold uppercase tracking-wider text-white">
                3. Apps & Websites Catalog
              </label>
              <button
                type="button"
                onClick={handleSelectAllCatalog}
                className="text-[11px] font-semibold text-[#A1A1AA] hover:text-white transition underline"
              >
                {selectedCatalogIds.length === APP_CATALOG.length ? "Deselect All" : "Select All"}
              </button>
            </div>
            <p className="text-[11px] text-[#A1A1AA] mb-3">
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
                    className={`flex items-center justify-between rounded-xl border p-2.5 text-xs font-semibold transition-all ${
                      isSelected
                        ? "border-white bg-white text-black font-bold shadow-sm"
                        : "border-white/10 bg-[#0E0E12] text-[#A1A1AA] hover:border-white/25 hover:text-white"
                    }`}
                  >
                    <span>{item.name}</span>
                    <Lock size={11} className={isSelected ? "text-black" : "text-neutral-500"} />
                  </button>
                );
              })}
            </div>
          </div>

          {/* 4. Custom Target Option */}
          <div className="rounded-xl border border-[#2B2B2B] bg-[#141414] p-3.5">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Plus size={14} className="text-[#A3A3A3]" />
                <span className="text-xs font-bold text-white">Custom Target</span>
              </div>
              <button
                type="button"
                onClick={() => setShowCustomInput((v) => !v)}
                className="text-[11px] font-semibold text-[#A3A3A3] hover:text-white transition underline"
              >
                {showCustomInput ? "Hide" : "+ Add Custom Package / Domain"}
              </button>
            </div>

            {showCustomInput && (
              <div className="mt-3 space-y-2 border-t border-[#262626] pt-3">
                <div className="flex items-center gap-2 text-[11px]">
                  <button
                    type="button"
                    onClick={() => setCustomType("app")}
                    className={`flex items-center gap-1 rounded-lg px-2.5 py-1 font-semibold transition ${
                      customType === "app"
                        ? "bg-white text-black font-bold"
                        : "bg-[#222222] text-[#A3A3A3] hover:text-white"
                    }`}
                  >
                    <Smartphone size={11} />
                    <span>Android Package</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => setCustomType("domain")}
                    className={`flex items-center gap-1 rounded-lg px-2.5 py-1 font-semibold transition ${
                      customType === "domain"
                        ? "bg-white text-black font-bold"
                        : "bg-[#222222] text-[#A3A3A3] hover:text-white"
                    }`}
                  >
                    <Globe size={11} />
                    <span>Domain / URL</span>
                  </button>
                </div>

                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    value={customInput}
                    onChange={(e) => setCustomInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        handleAddCustomTarget();
                      }
                    }}
                    placeholder={
                      customType === "app"
                        ? "e.g. com.snapchat.android"
                        : "e.g. news.ycombinator.com"
                    }
                    className="flex-1 rounded-xl border border-[#333333] bg-[#121212] px-3 py-2 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none transition"
                  />
                  <button
                    type="button"
                    onClick={handleAddCustomTarget}
                    className="rounded-xl bg-[#262626] px-3 py-2 text-xs font-semibold text-white hover:bg-white hover:text-black transition"
                  >
                    Add
                  </button>
                </div>
              </div>
            )}

            {customTargets.length > 0 && (
              <div className="mt-3 flex flex-wrap gap-1.5 border-t border-[#262626] pt-3">
                {customTargets.map((ct) => (
                  <span
                    key={ct.id}
                    className="inline-flex items-center gap-1.5 rounded-lg border border-[#3A3A3A] bg-[#1A1A1A] px-2.5 py-1 text-[11px] text-neutral-300"
                  >
                    {ct.type === "app" ? (
                      <Smartphone size={10} className="text-neutral-400" />
                    ) : (
                      <Globe size={10} className="text-neutral-400" />
                    )}
                    <span>{ct.value}</span>
                    <button
                      type="button"
                      onClick={() => handleRemoveCustomTarget(ct.id)}
                      className="text-[#737373] hover:text-white transition"
                    >
                      <Trash2 size={10} />
                    </button>
                  </span>
                ))}
              </div>
            )}
          </div>

          {/* 5. Accountability Verification */}
          <div className="rounded-xl border border-white/10 bg-[#0E0E12] p-4 space-y-3">
            <div className="flex items-center justify-between">
              <label className="block text-xs font-bold uppercase tracking-wider text-white">
                5. Accountability Verification
              </label>
              {hasTargets ? (
                <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded bg-white text-black font-mono">
                  Locked: Friend Only
                </span>
              ) : (
                <span className="text-[10px] text-[#A1A1AA]">
                  Choose verification mode
                </span>
              )}
            </div>

            {hasTargets ? (
              <p className="text-[11px] text-[#A1A1AA] leading-relaxed">
                Goals with distraction locks require friend verification. Only an assigned accountability partner can review your proof and unlock your targets.
              </p>
            ) : (
              <div className="grid grid-cols-3 gap-2">
                <button
                  type="button"
                  onClick={() => setApprovalType("friend")}
                  className={`rounded-xl border p-2.5 text-xs font-semibold transition ${
                    approvalType === "friend"
                      ? "border-white bg-white text-black font-bold shadow-sm"
                      : "border-white/10 bg-[#161618] text-[#A1A1AA] hover:text-white"
                  }`}
                >
                  Friend
                </button>
                <button
                  type="button"
                  onClick={() => setApprovalType("community")}
                  className={`rounded-xl border p-2.5 text-xs font-semibold transition ${
                    approvalType === "community"
                      ? "border-white bg-white text-black font-bold shadow-sm"
                      : "border-white/10 bg-[#161618] text-[#A1A1AA] hover:text-white"
                  }`}
                >
                  Community
                </button>
                <button
                  type="button"
                  onClick={() => setApprovalType("none")}
                  className={`rounded-xl border p-2.5 text-xs font-semibold transition ${
                    approvalType === "none"
                      ? "border-white bg-white text-black font-bold shadow-sm"
                      : "border-white/10 bg-[#161618] text-[#A1A1AA] hover:text-white"
                  }`}
                >
                  None
                </button>
              </div>
            )}

            {(hasTargets || approvalType === "friend") && (
              <div className="space-y-1.5 pt-1">
                <label className="block text-xs font-semibold text-neutral-300">
                  Accountability Partner (@handle or email) <span className="text-white">*</span>
                </label>
                <input
                  type="text"
                  required={hasTargets || approvalType === "friend"}
                  placeholder="e.g. @partner or friend@example.com"
                  value={approverEmail}
                  onChange={(e) => setApproverEmail(e.target.value)}
                  className="w-full rounded-xl border border-white/10 bg-[#141418] px-3.5 py-2.5 text-xs text-white placeholder-neutral-500 focus:border-white/40 focus:ring-2 focus:ring-white/10 focus:outline-none transition"
                />
                <p className="text-[10px] text-neutral-400">
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
              className="btn-premium flex-1 rounded-xl border border-white/10 bg-white/5 py-3 text-xs font-semibold text-neutral-300 hover:text-white hover:bg-white/10 transition"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={submitting || !taskDescription.trim()}
              className="btn-glow flex-1 flex items-center justify-center gap-2 rounded-xl py-3 text-xs font-black uppercase tracking-wider text-black shadow-lg disabled:opacity-40"
            >
              {submitting ? (
                <>
                  <Loader2 size={15} className="animate-spin text-black" />
                  <span>Activating Lock...</span>
                </>
              ) : (
                <>
                  <Lock size={14} strokeWidth={2.5} />
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
