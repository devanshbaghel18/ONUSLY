import { useState, useEffect } from "react";
import { Lock, X, Loader2 } from "lucide-react";
import { createGoal } from "../lib/api";
import { APPS_TO_BLOCK, saveGoalBlockedApps } from "../lib/blockedApps";

export default function CreateGoalModal({ isOpen, onClose, onGoalCreated }) {
  const [taskDescription, setTaskDescription] = useState("");
  const [taskDetails, setTaskDetails] = useState("");
  const [selectedApps, setSelectedApps] = useState(["youtube", "instagram", "twitter"]);
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState("");

  useEffect(() => {
    if (isOpen) {
      const timer = setTimeout(() => {
        setTaskDescription("");
        setTaskDetails("");
        setSelectedApps(["youtube", "instagram", "twitter"]);
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

  const toggleApp = (appId) => {
    setSelectedApps((prev) =>
      prev.includes(appId)
        ? prev.filter((id) => id !== appId)
        : [...prev, appId]
    );
  };

  const handleSelectAllApps = () => {
    if (selectedApps.length === APPS_TO_BLOCK.length) {
      setSelectedApps([]);
    } else {
      setSelectedApps(APPS_TO_BLOCK.map((a) => a.id));
    }
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setFormError("");

    if (!taskDescription.trim()) {
      setFormError("Goal name / task description is required");
      return;
    }

    setSubmitting(true);
    try {
      const created = await createGoal({
        title: taskDescription.trim(),
        description: taskDetails.trim(),
      });

      // Save blocked apps mapping for this goal
      saveGoalBlockedApps(created.id, selectedApps);

      if (onGoalCreated) {
        onGoalCreated(created);
      }
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
                Lock distracting apps until you submit proof in chat.
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

          {/* 3. Choose which apps to lock */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="block text-xs font-bold uppercase tracking-wider text-white">
                3. Select Apps to Block
              </label>
              <button
                type="button"
                onClick={handleSelectAllApps}
                className="text-[11px] font-semibold text-[#A3A3A3] hover:text-white transition underline"
              >
                {selectedApps.length === APPS_TO_BLOCK.length ? "Deselect All" : "Select All"}
              </button>
            </div>
            <p className="text-[11px] text-[#737373] mb-3">
              These distracting apps will be blocked while this lock is active ({selectedApps.length} selected):
            </p>

            <div className="flex flex-wrap gap-2">
              {APPS_TO_BLOCK.map((app) => {
                const isSelected = selectedApps.includes(app.id);
                return (
                  <button
                    key={app.id}
                    type="button"
                    onClick={() => toggleApp(app.id)}
                    className={`flex items-center gap-1.5 rounded-xl border px-3 py-2 text-xs font-semibold transition-all ${
                      isSelected
                        ? "border-white bg-white text-black font-bold shadow-sm"
                        : "border-[#333333] bg-[#121212] text-[#A3A3A3] hover:border-[#555555] hover:text-white"
                    }`}
                  >
                    <Lock size={11} className={isSelected ? "text-black" : "text-[#737373]"} />
                    <span>{app.label}</span>
                  </button>
                );
              })}
            </div>
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
