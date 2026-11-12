import { useState, useEffect, useRef } from "react";
import {
  X,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Loader2,
  Lock,
  Unlock,
  MessageSquare,
  Sparkles,
} from "lucide-react";
import { getUniqueDisplayTargets } from "../../lib/blockedApps";

export default function DecisionModal({
  isOpen,
  type = "approve", // "approve" | "reject"
  item,
  onClose,
  onConfirm,
}) {
  const [comment, setComment] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const textareaRef = useRef(null);

  const isApprove = type === "approve";
  const minLength = 5;
  const currentLength = comment.trim().length;
  const isCommentValid = isApprove || currentLength >= minLength;

  // Reset state when opening
  useEffect(() => {
    if (isOpen) {
      setComment(isApprove ? "Approved verification proof." : "");
      setSubmitting(false);
      setError("");
      // Auto focus textarea on rejection
      if (!isApprove) {
        setTimeout(() => textareaRef.current?.focus(), 50);
      }
    }
  }, [isOpen, isApprove]);

  // Handle escape key
  useEffect(() => {
    if (!isOpen || submitting) return;
    const handleKeyDown = (e) => {
      if (e.key === "Escape") {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, submitting, onClose]);

  if (!isOpen || !item) return null;

  const displayTargets = getUniqueDisplayTargets(item.blockedApps, item.blockedDomains);
  const partnerName = item.ownerName || item.ownerHandle || "your partner";

  const handleSubmit = async (e) => {
    e?.preventDefault();
    if (!isCommentValid || submitting) return;

    setSubmitting(true);
    setError("");

    try {
      await onConfirm({
        status: isApprove ? "approved" : "rejected",
        comment: comment.trim(),
      });
      onClose();
    } catch (err) {
      setError(err?.message || `Failed to submit ${type} decision. Please try again.`);
      setSubmitting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm animate-in fade-in duration-150"
      onClick={(e) => {
        if (e.target === e.currentTarget && !submitting) onClose();
      }}
    >
      <div className="relative w-full max-w-lg rounded-2xl border border-white/10 bg-[#121217] p-6 shadow-2xl animate-in zoom-in-95 duration-150">
        {/* Close Button */}
        <button
          type="button"
          onClick={onClose}
          disabled={submitting}
          className="absolute right-4 top-4 rounded-lg p-1.5 text-neutral-400 hover:bg-white/5 hover:text-white disabled:opacity-40"
        >
          <X size={18} />
        </button>

        {/* Modal Header */}
        <div className="flex items-start gap-4">
          <div
            className={`flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl border ${
              isApprove
                ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-400"
                : "border-rose-500/30 bg-rose-500/10 text-rose-400"
            }`}
          >
            {isApprove ? <CheckCircle2 size={24} /> : <AlertTriangle size={24} />}
          </div>
          <div className="min-w-0 pr-6">
            <h2 className="text-lg font-bold text-white tracking-tight">
              {isApprove ? "Approve Goal & Unlock Targets" : "Reject Proof Submission"}
            </h2>
            <p className="mt-0.5 text-xs text-neutral-400">
              {isApprove
                ? `Verify that ${partnerName} successfully completed their goal.`
                : `Explain why this submission does not satisfy requirements.`}
            </p>
          </div>
        </div>

        {/* Goal Context Summary */}
        <div className="mt-4 rounded-xl border border-white/5 bg-white/[0.02] p-3 text-xs space-y-1.5">
          <div className="flex items-center gap-2">
            <span className="font-semibold text-white truncate">{item.title}</span>
            <span className="text-neutral-500">•</span>
            <span className="text-neutral-400">From {partnerName}</span>
          </div>

          {displayTargets.length > 0 && (
            <div className="flex flex-wrap items-center gap-1 pt-1">
              <span className="text-[11px] text-neutral-500 flex items-center gap-1 mr-1">
                {isApprove ? <Unlock size={11} className="text-emerald-400" /> : <Lock size={11} className="text-rose-400" />}
                {isApprove ? "Unlocks:" : "Remains Locked:"}
              </span>
              {displayTargets.map((target, idx) => (
                <span
                  key={idx}
                  className="rounded-md border border-white/10 bg-white/5 px-1.5 py-0.5 text-[11px] text-neutral-300"
                >
                  {target.label}
                </span>
              ))}
            </div>
          )}
        </div>

        {/* Body & Input */}
        <form onSubmit={handleSubmit} className="mt-4 space-y-4">
          {/* Impact Banner */}
          {isApprove ? (
            <div className="rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-3 text-xs leading-relaxed text-emerald-300">
              ✨ <strong>Unlocking Action</strong>: Approving this proof will mark the goal as complete and instantly unlock all restricted apps and websites.
            </div>
          ) : (
            <div className="rounded-xl border border-rose-500/20 bg-rose-500/5 p-3 text-xs leading-relaxed text-rose-300">
              🔒 <strong>Accountability Lock</strong>: Targets will remain strictly locked. Your partner must read your feedback and submit a new proof.
            </div>
          )}

          {/* Comment / Feedback Input */}
          <div className="space-y-1.5">
            <div className="flex items-center justify-between text-xs">
              <label
                htmlFor="decision-comment"
                className="font-medium text-neutral-300 flex items-center gap-1.5"
              >
                <MessageSquare size={13} className="text-neutral-400" />
                {isApprove ? "Encouragement note (optional):" : "Rejection feedback (required):"}
              </label>

              {!isApprove && (
                <span
                  className={`font-mono text-[11px] font-semibold ${
                    currentLength >= minLength ? "text-emerald-400" : "text-amber-400"
                  }`}
                >
                  {currentLength >= minLength ? (
                    `✓ ${currentLength} characters`
                  ) : (
                    `${currentLength}/${minLength} min chars`
                  )}
                </span>
              )}
            </div>

            <textarea
              id="decision-comment"
              ref={textareaRef}
              rows={3}
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              placeholder={
                isApprove
                  ? "e.g. Great job staying focused! Proof verified."
                  : "e.g. The photo is too blurry to read the chapter notes, or please provide the study timer screenshot..."
              }
              className="w-full rounded-xl border border-white/10 bg-white/[0.03] p-3 text-xs text-white placeholder-neutral-500 transition-all focus:border-white/30 focus:outline-none focus:ring-1 focus:ring-white/20 resize-none"
            />
          </div>

          {/* Error Message */}
          {error && (
            <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-300">
              {error}
            </div>
          )}

          {/* Action Buttons */}
          <div className="flex items-center justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="rounded-xl border border-white/10 bg-white/5 px-4 py-2.5 text-xs font-medium text-neutral-300 transition-all hover:bg-white/10 hover:text-white disabled:opacity-50"
            >
              Cancel
            </button>

            <button
              type="submit"
              disabled={!isCommentValid || submitting}
              className={`inline-flex items-center justify-center gap-2 rounded-xl px-5 py-2.5 text-xs font-semibold text-white shadow-lg transition-all disabled:cursor-not-allowed disabled:opacity-40 active:scale-[0.98] ${
                isApprove
                  ? "bg-emerald-600 hover:bg-emerald-500 shadow-emerald-950/40"
                  : "bg-rose-600 hover:bg-rose-500 shadow-rose-950/40"
              }`}
            >
              {submitting ? (
                <>
                  <Loader2 size={14} className="animate-spin" />
                  <span>Submitting...</span>
                </>
              ) : isApprove ? (
                <>
                  <CheckCircle2 size={14} />
                  <span>Confirm Approval</span>
                </>
              ) : (
                <>
                  <XCircle size={14} />
                  <span>Confirm Rejection</span>
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
