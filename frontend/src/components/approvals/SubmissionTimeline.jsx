import { useState } from "react";
import {
  CheckCircle2,
  XCircle,
  Clock,
  AlertTriangle,
  ChevronDown,
  ChevronUp,
  FileText,
  Image,
  ExternalLink,
  ShieldCheck,
  File,
  Copy,
  Check,
  Flame,
  Target,
  Zap,
  Award,
} from "lucide-react";
import { API_BASE_URL } from "../../lib/api";

function resolveMediaUrl(rawUrl) {
  if (!rawUrl) return "";
  if (
    rawUrl.startsWith("data:") ||
    rawUrl.startsWith("blob:") ||
    rawUrl.startsWith("http://") ||
    rawUrl.startsWith("https://")
  ) {
    return rawUrl;
  }
  const cleanBase = (
    API_BASE_URL ||
    (typeof window !== "undefined" && window.location
      ? `${window.location.protocol}//${window.location.hostname}:8080`
      : "http://localhost:8080")
  ).replace(/\/+$/, "");
  return `${cleanBase}${rawUrl.startsWith("/") ? "" : "/"}${rawUrl}`;
}

function formatRelativeTime(dateString) {
  if (!dateString) return "";
  const date = new Date(dateString);
  if (isNaN(date.getTime())) return dateString;

  const diffMs = Date.now() - date.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHr = Math.floor(diffMin / 60);
  const diffDays = Math.floor(diffHr / 24);

  if (diffSec < 60) return "just now";
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHr < 24) return `${diffHr}h ago`;
  if (diffDays === 1) return "yesterday";
  if (diffDays < 7) return `${diffDays}d ago`;

  return date.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export default function SubmissionTimeline({
  attempts = [],
  auditLog = [],
  accountabilityMetrics = null,
  onSelectProof = null,
  isOwner = false,
  approverEmail = "",
}) {
  const [copiedReceiptId, setCopiedReceiptId] = useState("");
  const [showAuditDrawer, setShowAuditDrawer] = useState(false);

  const [expandedAttempts, setExpandedAttempts] = useState(() => {
    const initial = {};
    if (attempts.length > 0) {
      initial[attempts.length - 1] = true;
    }
    return initial;
  });

  const toggleAttempt = (idx) => {
    setExpandedAttempts((prev) => ({
      ...prev,
      [idx]: !prev[idx],
    }));
  };

  const expandAll = () => {
    const all = {};
    attempts.forEach((_, i) => {
      all[i] = true;
    });
    setExpandedAttempts(all);
  };

  const collapseAll = () => {
    const latestOnly = {};
    if (attempts.length > 0) {
      latestOnly[attempts.length - 1] = true;
    }
    setExpandedAttempts(latestOnly);
  };

  const handleCopyReceipt = (id) => {
    if (!id) return;
    navigator.clipboard?.writeText(id);
    setCopiedReceiptId(id);
    setTimeout(() => setCopiedReceiptId(""), 2000);
  };

  if (!attempts || attempts.length === 0) {
    return (
      <div className="rounded-2xl border border-white/10 bg-[#1E1E1E] p-6 text-center text-neutral-400">
        <Clock className="mx-auto mb-2 text-neutral-500" size={24} />
        <p className="text-sm font-medium">No verification attempts submitted yet.</p>
        <p className="text-xs text-neutral-500 mt-1">
          When proof is submitted, the full multi-attempt audit history will appear here.
        </p>
      </div>
    );
  }

  const reversedAttempts = [...attempts].reverse();

  return (
    <div className="space-y-5">
      {/* 5.4 Accountability Streak & Velocity Metrics Strip */}
      {accountabilityMetrics && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {/* Active Streak */}
          <div className="rounded-xl border border-amber-500/20 bg-gradient-to-br from-amber-500/10 to-transparent p-3 shadow-sm">
            <div className="flex items-center gap-1.5 text-xs font-semibold text-amber-400">
              <Flame size={14} className="fill-amber-400 text-amber-400" />
              <span>Streak</span>
            </div>
            <div className="mt-1 flex items-baseline gap-1">
              <span className="text-xl font-extrabold text-white">
                {accountabilityMetrics.currentStreak || 0}
              </span>
              <span className="text-[11px] text-amber-200/70 font-medium">
                {accountabilityMetrics.currentStreak === 1 ? "Goal" : "Goals"}
              </span>
            </div>
            <p className="text-[10px] text-neutral-400 mt-0.5">Consecutive completion</p>
          </div>

          {/* Verification Success Rate */}
          <div className="rounded-xl border border-emerald-500/20 bg-gradient-to-br from-emerald-500/10 to-transparent p-3 shadow-sm">
            <div className="flex items-center gap-1.5 text-xs font-semibold text-emerald-400">
              <Target size={14} />
              <span>Accuracy</span>
            </div>
            <div className="mt-1 flex items-baseline gap-1">
              <span className="text-xl font-extrabold text-white">
                {Math.round(accountabilityMetrics.completionRate || 100)}%
              </span>
              <span className="text-[11px] text-emerald-200/70 font-medium">Success</span>
            </div>
            <p className="text-[10px] text-neutral-400 mt-0.5">Verified vs rejected</p>
          </div>

          {/* Review Turnaround Time */}
          <div className="rounded-xl border border-sky-500/20 bg-gradient-to-br from-sky-500/10 to-transparent p-3 shadow-sm">
            <div className="flex items-center gap-1.5 text-xs font-semibold text-sky-400">
              <Zap size={14} />
              <span>Velocity</span>
            </div>
            <div className="mt-1 flex items-baseline gap-1">
              <span className="text-xl font-extrabold text-white">
                {accountabilityMetrics.avgReviewTimeMinutes > 0
                  ? `${accountabilityMetrics.avgReviewTimeMinutes}m`
                  : "<15m"}
              </span>
              <span className="text-[11px] text-sky-200/70 font-medium">Avg Review</span>
            </div>
            <p className="text-[10px] text-neutral-400 mt-0.5">Partner response time</p>
          </div>

          {/* Total Verified Goals */}
          <div className="rounded-xl border border-purple-500/20 bg-gradient-to-br from-purple-500/10 to-transparent p-3 shadow-sm">
            <div className="flex items-center gap-1.5 text-xs font-semibold text-purple-400">
              <Award size={14} />
              <span>Verified</span>
            </div>
            <div className="mt-1 flex items-baseline gap-1">
              <span className="text-xl font-extrabold text-white">
                {accountabilityMetrics.totalVerified || 0}
              </span>
              <span className="text-[11px] text-purple-200/70 font-medium">Unlocked</span>
            </div>
            <p className="text-[10px] text-neutral-400 mt-0.5">Total verified milestones</p>
          </div>
        </div>
      )}

      {/* Header with Total Counter & Controls */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-white/10 pb-3">
        <div className="flex items-center gap-2">
          <span className="text-sm font-bold text-white">
            Submission History & Decision Audit
          </span>
          <span className="rounded-full bg-white/10 px-2 py-0.5 text-xs font-semibold text-neutral-300">
            {attempts.length} {attempts.length === 1 ? "Attempt" : "Attempts"}
          </span>
        </div>

        <div className="flex items-center gap-3 text-xs">
          {auditLog.length > 0 && (
            <button
              type="button"
              onClick={() => setShowAuditDrawer(!showAuditDrawer)}
              className="flex items-center gap-1.5 rounded-lg border border-white/10 bg-white/5 px-2.5 py-1 text-neutral-300 hover:bg-white/10 hover:text-white transition"
            >
              <ShieldCheck size={13} className="text-indigo-400" />
              <span>{showAuditDrawer ? "Hide Audit Log" : `Audit Receipts (${auditLog.length})`}</span>
            </button>
          )}

          {attempts.length > 1 && (
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={expandAll}
                className="text-neutral-400 hover:text-white transition"
              >
                Expand All
              </button>
              <span className="text-neutral-600">•</span>
              <button
                type="button"
                onClick={collapseAll}
                className="text-neutral-400 hover:text-white transition"
              >
                Collapse Past
              </button>
            </div>
          )}
        </div>
      </div>

      {/* Immutable Audit Receipts Panel */}
      {showAuditDrawer && auditLog.length > 0 && (
        <div className="rounded-2xl border border-indigo-500/30 bg-gradient-to-b from-[#161622] to-[#12121A] p-4 space-y-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2 text-xs font-bold text-indigo-300">
              <ShieldCheck size={16} />
              <span>Immutable Verification Audit Receipts</span>
            </div>
            <span className="text-[10px] text-neutral-400 uppercase tracking-wider">
              Cryptographically Recorded
            </span>
          </div>

          <div className="space-y-2">
            {auditLog.map((receipt, idx) => (
              <div
                key={receipt.receiptId || idx}
                className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-white/5 bg-black/40 p-3 text-xs"
              >
                <div className="space-y-1">
                  <div className="flex items-center gap-2">
                    <span
                      className={`inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-bold uppercase ${
                        receipt.decision === "approved"
                          ? "bg-emerald-500/20 text-emerald-300"
                          : "bg-amber-500/20 text-amber-300"
                      }`}
                    >
                      {receipt.decision}
                    </span>
                    <span className="font-semibold text-white">
                      Attempt #{receipt.attemptIndex} Decision
                    </span>
                    <span className="text-[11px] text-neutral-400">
                      • {formatRelativeTime(receipt.decidedAt)}
                    </span>
                  </div>

                  <p className="text-[11px] text-neutral-300">
                    Decision by{" "}
                    <strong className="text-white">
                      {receipt.approverName || approverEmail || receipt.approverId}
                    </strong>
                    {receipt.comment && ` — "${receipt.comment}"`}
                  </p>
                </div>

                <div className="flex items-center gap-2">
                  <code className="rounded bg-white/5 px-2 py-1 font-mono text-[10px] text-neutral-400">
                    ID: {receipt.receiptId?.slice(0, 14)}...
                  </code>
                  <button
                    type="button"
                    onClick={() => handleCopyReceipt(receipt.receiptId)}
                    className="rounded p-1 text-neutral-400 hover:bg-white/10 hover:text-white transition"
                    title="Copy full receipt ID"
                  >
                    {copiedReceiptId === receipt.receiptId ? (
                      <Check size={13} className="text-emerald-400" />
                    ) : (
                      <Copy size={13} />
                    )}
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Vertical Stepper List */}
      <div className="relative pl-6 space-y-6 before:absolute before:bottom-3 before:left-2.5 before:top-3 before:w-0.5 before:bg-white/10">
        {reversedAttempts.map((attempt, revIdx) => {
          const originalIdx = attempts.length - 1 - revIdx;
          const isLatest = originalIdx === attempts.length - 1;
          const isExpanded = Boolean(expandedAttempts[originalIdx]);

          const isApproved = attempt.status === "approved";
          const isRejected = attempt.status === "rejected";
          const isPending = attempt.status === "pending" || attempt.status === "proof_submitted";

          return (
            <div key={attempt.proofId || originalIdx} className="relative group">
              {/* Step Node Dot on Timeline */}
              <div
                className={`absolute -left-6 top-3 flex h-5 w-5 items-center justify-center rounded-full border-2 transition-all ${
                  isApproved
                    ? "border-emerald-500 bg-emerald-950 text-emerald-400 ring-2 ring-emerald-500/20"
                    : isRejected
                    ? "border-amber-500 bg-amber-950 text-amber-400 ring-2 ring-amber-500/20"
                    : "border-sky-500 bg-sky-950 text-sky-400 ring-2 ring-sky-500/20 animate-pulse"
                }`}
              >
                {isApproved ? (
                  <CheckCircle2 size={12} className="stroke-[3]" />
                ) : isRejected ? (
                  <XCircle size={12} className="stroke-[3]" />
                ) : (
                  <Clock size={12} className="stroke-[3]" />
                )}
              </div>

              {/* Attempt Card Container */}
              <div
                className={`rounded-2xl border transition-all ${
                  isLatest
                    ? "border-white/20 bg-neutral-900/90 shadow-md ring-1 ring-white/10"
                    : "border-white/10 bg-[#1A1A1A]/90 hover:border-white/20"
                }`}
              >
                {/* Accordion Header */}
                <button
                  type="button"
                  onClick={() => toggleAttempt(originalIdx)}
                  className="flex w-full items-center justify-between p-4 text-left focus:outline-none"
                >
                  <div className="flex flex-wrap items-center gap-2.5">
                    <span className="text-xs font-bold text-white uppercase tracking-wider">
                      Attempt #{attempt.attemptNumber}
                    </span>

                    {isLatest && (
                      <span className="rounded-md bg-white px-2 py-0.5 text-[10px] font-extrabold text-black uppercase">
                        Current
                      </span>
                    )}

                    {/* Status Pill */}
                    <span
                      className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${
                        isApproved
                          ? "bg-emerald-500/15 text-emerald-300 border border-emerald-500/30"
                          : isRejected
                          ? "bg-amber-500/15 text-amber-300 border border-amber-500/30"
                          : "bg-sky-500/15 text-sky-300 border border-sky-500/30"
                      }`}
                    >
                      {isApproved && "Approved"}
                      {isRejected && "Rejected"}
                      {isPending && "In Review"}
                    </span>

                    <span className="text-[11px] text-neutral-400">
                      • {formatRelativeTime(attempt.submittedAt)}
                    </span>
                  </div>

                  <div className="flex items-center gap-2 text-neutral-400">
                    {isExpanded ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
                  </div>
                </button>

                {/* Collapsible Attempt Body */}
                {isExpanded && (
                  <div className="px-4 pb-4 pt-1 space-y-3.5 border-t border-white/5">
                    {/* User Explanation */}
                    {attempt.explanation && (
                      <div className="rounded-xl border border-white/5 bg-neutral-950/70 p-3">
                        <span className="text-[10px] font-bold uppercase tracking-wider text-neutral-400 block mb-1">
                          Owner Note
                        </span>
                        <p className="text-xs text-neutral-200 whitespace-pre-wrap leading-relaxed">
                          {attempt.explanation}
                        </p>
                      </div>
                    )}

                    {/* Verification Link */}
                    {attempt.externalLink && (
                      <div className="flex items-center gap-2 text-xs">
                        <ExternalLink size={14} className="text-neutral-400 shrink-0" />
                        <span className="text-neutral-400">Link:</span>
                        <a
                          href={
                            attempt.externalLink.startsWith("http")
                              ? attempt.externalLink
                              : `https://${attempt.externalLink}`
                          }
                          target="_blank"
                          rel="noreferrer"
                          className="text-white underline hover:text-neutral-300 truncate font-mono"
                        >
                          {attempt.externalLink}
                        </a>
                      </div>
                    )}

                    {/* Media / Files Preview Chips */}
                    {attempt.photoUrl && (
                      <div className="space-y-1.5">
                        <span className="text-[10px] font-bold uppercase tracking-wider text-neutral-400 block">
                          Submitted Evidence
                        </span>
                        {attempt.photoUrl.startsWith("data:image/") ||
                        attempt.photoUrl.endsWith(".png") ||
                        attempt.photoUrl.endsWith(".jpg") ||
                        attempt.photoUrl.endsWith(".jpeg") ||
                        attempt.photoUrl.endsWith(".webp") ? (
                          <div className="relative group/img max-w-sm">
                            <img
                              src={resolveMediaUrl(attempt.photoUrl)}
                              alt={`Evidence Attempt #${attempt.attemptNumber}`}
                              className="max-h-48 rounded-xl object-cover border border-white/10 shadow-sm"
                            />
                            {onSelectProof && (
                              <button
                                type="button"
                                onClick={() => onSelectProof(attempt)}
                                className="mt-1.5 inline-flex items-center gap-1.5 text-xs text-neutral-400 hover:text-white transition"
                              >
                                <Image size={13} />
                                <span>Inspect in full lightbox</span>
                              </button>
                            )}
                          </div>
                        ) : (
                          <div className="flex items-center gap-2 rounded-lg border border-white/10 bg-neutral-950 p-2 text-xs text-neutral-300 max-w-md">
                            <File size={14} className="text-neutral-400 shrink-0" />
                            <span className="truncate">{attempt.photoUrl}</span>
                          </div>
                        )}
                      </div>
                    )}

                    {/* Decision Outcome Card */}
                    {isRejected && (
                      <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-3.5 space-y-2">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-2 text-xs font-bold text-amber-300">
                            <AlertTriangle size={15} className="shrink-0" />
                            <span>Partner Rejection Feedback</span>
                          </div>
                          {attempt.decidedAt && (
                            <span className="text-[10px] text-amber-400/80">
                              {formatRelativeTime(attempt.decidedAt)}
                            </span>
                          )}
                        </div>

                        {attempt.comment ? (
                          <p className="text-xs text-amber-100 bg-amber-950/40 border border-amber-500/20 rounded-lg p-2.5 font-medium leading-relaxed">
                            "{attempt.comment}"
                          </p>
                        ) : (
                          <p className="text-xs text-amber-200/80 italic">
                            No written reason provided by approver.
                          </p>
                        )}

                        <div className="text-[10px] text-amber-400/80 flex items-center gap-1.5 pt-0.5">
                          <ShieldCheck size={12} />
                          <span>
                            Reviewed by {attempt.approverName || approverEmail || "Partner"}
                          </span>
                        </div>
                      </div>
                    )}

                    {isApproved && (
                      <div className="rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3.5 space-y-1.5">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-2 text-xs font-bold text-emerald-300">
                            <CheckCircle2 size={15} className="shrink-0" />
                            <span>Verified & Approved</span>
                          </div>
                          {attempt.decidedAt && (
                            <span className="text-[10px] text-emerald-400/80">
                              {formatRelativeTime(attempt.decidedAt)}
                            </span>
                          )}
                        </div>
                        {attempt.comment && (
                          <p className="text-xs text-emerald-100 pl-6">
                            "{attempt.comment}"
                          </p>
                        )}
                        <div className="text-[10px] text-emerald-400/80 flex items-center gap-1.5 pl-6 pt-0.5">
                          <ShieldCheck size={12} />
                          <span>
                            Verified by {attempt.approverName || approverEmail || "Partner"}
                          </span>
                        </div>
                      </div>
                    )}

                    {isPending && (
                      <div className="flex items-center gap-2 rounded-xl border border-sky-500/20 bg-sky-500/5 p-3 text-xs text-sky-300">
                        <Clock size={14} className="shrink-0 animate-spin" />
                        <span>
                          Pending partner verification. Target restrictions remain active.
                        </span>
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
