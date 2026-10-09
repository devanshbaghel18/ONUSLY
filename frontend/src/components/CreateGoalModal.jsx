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
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState("");

  useEffect(() => {
    if (isOpen) {
      const timer = setTimeout(() => {
        setTaskDescription("");
        setTaskDetails("");
        setSelectedCatalogIds(["youtube", "instagram", "twitter"]);
        setCustomTargets([]);
        setCustomInput("");
        setShowCustomInput(false);
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

    if (customTargets.some((t) => t.value.toLowerCase() === trimmed.toLowerCase())) {
      setCustomInput("");
      return;
    }

    setCustomTargets((prev) => [
      ...prev,
      {
        id: `custom-${Date.now()}-${Math.random()}`,
        type: customType,
        value: trimmed,
      },
    ]);
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
        (item.domains || []).forEach((d) => domainsSet.add(d));
      }
    });

    customTargets.forEach((ct) => {
      if (ct.type === "app") {
        appsSet.add(ct.value);
      } else {
        domainsSet.add(ct.value);
      }
    });

    setSubmitting(true);
    try {
      const created = await createGoal({
        title: taskDescription.trim(),
        description: taskDetails.trim(),
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
      setFormError(err.message || "Failed to start accountability lock");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/85 p-4 backdrop-blur-md"
      onClick={onClose}
    >
      <div
        className="w-full max-w-lg rounded-2xl border border-[#333333] bg-[#181818] p-6 sm:p-7 shadow-2xl max-h-[90vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-[#2A2A2A] pb-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-white text-black shadow-sm">
              <Lock size={18} />
            </div>
            <div>
              <h3 className="text-base font-bold text-white">
                New Accountability Lock
              </h3>
              <p className="text-xs text-[#A3A3A3]">
                Lock distracting apps and websites until approved.
              </p>
            </div>
          </div>

          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="rounded-lg p-1.5 text-[#737373] hover:text-white hover:bg-[#252525] transition"
          >
            <X size={18} />
          </button>
        </div>

        {formError && (
          <div className="mt-4 rounded-xl border border-red-900 bg-red-950/40 p-3 text-xs text-red-300">
            {formError}
          </div>
        )}

        <form onSubmit={handleSubmit} className="mt-5 space-y-5">
          {/* 1. Goal / Task Name */}
          <div>
            <label className="block text-xs font-bold uppercase tracking-wider text-white mb-1.5">
              1. Goal Name / Task <span className="text-red-400">*</span>
            </label>
            <input
              type="text"
              required
              autoFocus
              placeholder="e.g. Master DSA Graph Algorithms, Complete 5km Run..."
              value={taskDescription}
              onChange={(e) => setTaskDescription(e.target.value)}
              className="w-full rounded-xl border border-[#333333] bg-[#121212] px-4 py-3 text-sm text-white placeholder-[#737373] focus:border-white focus:outline-none transition"
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
              className="w-full rounded-xl border border-[#333333] bg-[#121212] px-4 py-2.5 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none transition resize-none"
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
                className="text-[11px] font-semibold text-[#A3A3A3] hover:text-white transition underline"
              >
                {selectedCatalogIds.length === APP_CATALOG.length ? "Deselect All" : "Select All"}
              </button>
            </div>
            <p className="text-[11px] text-[#737373] mb-3">
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
                        : "border-[#333333] bg-[#121212] text-[#A3A3A3] hover:border-[#555555] hover:text-white"
                    }`}
                  >
                    <span>{item.name}</span>
                    <Lock size={11} className={isSelected ? "text-black" : "text-[#737373]"} />
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
                      <Smartphone size={10} className="text-blue-400" />
                    ) : (
                      <Globe size={10} className="text-emerald-400" />
                    )}
                    <span>{ct.value}</span>
                    <button
                      type="button"
                      onClick={() => handleRemoveCustomTarget(ct.id)}
                      className="text-[#737373] hover:text-red-400 transition"
                    >
                      <Trash2 size={10} />
                    </button>
                  </span>
                ))}
              </div>
            )}
          </div>

          {/* Action buttons */}
          <div className="pt-2 flex items-center gap-3">
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="flex-1 rounded-xl border border-[#3A3A3A] bg-[#222222] py-3 text-xs font-semibold text-white hover:border-white transition"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={submitting || !taskDescription.trim()}
              className="flex-1 flex items-center justify-center gap-2 rounded-xl bg-white py-3 text-xs font-bold text-black transition-all hover:bg-neutral-200 disabled:opacity-40"
            >
              {submitting ? (
                <>
                  <Loader2 size={15} className="animate-spin" />
                  <span>Activating Lock...</span>
                </>
              ) : (
                <>
                  <Lock size={14} />
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
