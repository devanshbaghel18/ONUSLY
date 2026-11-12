import React from "react";
import {
  Lock,
  Unlock,
  CheckCircle2,
  XCircle,
  Clock,
  Camera,
  FileText,
  ExternalLink,
  Eye,
  AlertTriangle,
  Layers,
} from "lucide-react";
import { getUniqueDisplayTargets } from "../../lib/blockedApps";

function formatRelativeTime(dateString) {
  if (!dateString) return "";
  const date = new Date(dateString);
  if (isNaN(date.getTime())) return "";
  const now = new Date();
  const diffSec = Math.floor((now - date) / 1000);

  if (diffSec < 45) return "just now";
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHours = Math.floor(diffMin / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString([], { month: "short", day: "numeric" });
}

function getInitials(name, handle, email) {
  if (name && name.trim()) {
    const parts = name.trim().split(/\s+/);
    if (parts.length >= 2) {
      return (parts[0][0] + parts[1][0]).toUpperCase();
    }
    return parts[0].slice(0, 2).toUpperCase();
  }
  if (handle && handle.trim()) {
    return handle.replace(/^@/, "").slice(0, 2).toUpperCase();
  }
  if (email && email.trim()) {
    return email.slice(0, 2).toUpperCase();
  }
  return "AP";
}

export default function ApprovalCard({
  item,
  onInspectPhoto,
  onApprove,
  onReject,
}) {
  const {
    goalId,
    ownerName,
    ownerHandle,
    ownerEmail,
    title,
    description,
    status,
    createdAt,
    blockedApps = [],
    blockedDomains = [],
    latestProof,
    proofCount = 0,
  } = item;

  const initials = getInitials(ownerName, ownerHandle, ownerEmail);
  const displayTargets = getUniqueDisplayTargets(blockedApps, blockedDomains);
  const relativeTime = formatRelativeTime(latestProof?.submittedAt || createdAt);
  const isPending = status === "proof_submitted";
  const isApproved = status === "completed";
  const isRejected = status === "rejected";

  return (
    <div className="group relative overflow-hidden rounded-2xl border border-white/10 bg-[#121217] p-6 transition-all duration-200 hover:border-white/20 hover:shadow-xl hover:shadow-black/40">
      {/* Header Row: Owner info & Status badge */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-white/10 bg-gradient-to-br from-indigo-500/20 via-purple-500/10 to-transparent text-sm font-bold text-indigo-300">
            {initials}
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="font-semibold text-white">
                {ownerName || ownerHandle || ownerEmail || "Partner"}
              </h3>
              {(ownerHandle || ownerEmail) && (
                <span className="text-xs text-neutral-400">
                  {ownerHandle
                    ? `@${ownerHandle.replace(/^@/, "")}`
                    : ownerEmail}
                </span>
              )}
            </div>
            <p className="text-xs text-neutral-500">
              Submitted {relativeTime || "recently"}
            </p>
          </div>
        </div>

        {/* Status Pill */}
        <div className="flex items-center gap-2">
          {isPending && (
            <span className="inline-flex items-center gap-1.5 rounded-full border border-amber-500/30 bg-amber-500/10 px-3 py-1 text-xs font-semibold text-amber-300">
              <span className="h-1.5 w-1.5 rounded-full bg-amber-400 animate-pulse" />
              Pending Review
            </span>
          )}
          {isApproved && (
            <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-3 py-1 text-xs font-semibold text-emerald-300">
              <CheckCircle2 size={13} className="text-emerald-400" />
              Approved & Unlocked
            </span>
          )}
          {isRejected && (
            <span className="inline-flex items-center gap-1.5 rounded-full border border-rose-500/30 bg-rose-500/10 px-3 py-1 text-xs font-semibold text-rose-300">
              <XCircle size={13} className="text-rose-400" />
              Rejected
            </span>
          )}
          {!isPending && !isApproved && !isRejected && (
            <span className="inline-flex items-center gap-1.5 rounded-full border border-blue-500/30 bg-blue-500/10 px-3 py-1 text-xs font-semibold text-blue-300">
              <Clock size={13} className="text-blue-400" />
              In Progress
            </span>
          )}
        </div>
      </div>

      {/* Goal Details */}
      <div className="mt-4 rounded-xl border border-white/5 bg-white/[0.02] p-4">
        <div className="flex items-start gap-2.5">
          <div className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-lg border border-white/10 bg-white/5 text-neutral-300">
            {isApproved ? (
              <Unlock size={14} className="text-emerald-400" />
            ) : (
              <Lock size={14} className="text-neutral-400" />
            )}
          </div>
          <div className="flex-1">
            <h4 className="text-base font-semibold text-white tracking-tight">
              {title}
            </h4>
            {description && (
              <p className="mt-1 text-xs leading-relaxed text-neutral-400">
                {description}
              </p>
            )}

            {/* Blocked Targets */}
            {displayTargets.length > 0 && (
              <div className="mt-3 flex flex-wrap items-center gap-1.5">
                <span className="text-xs font-medium text-neutral-500 mr-1 flex items-center gap-1">
                  <Lock size={11} /> Blocked:
                </span>
                {displayTargets.map((target, idx) => (
                  <span
                    key={idx}
                    className="inline-flex items-center rounded-md border border-white/10 bg-white/5 px-2 py-0.5 text-xs font-medium text-neutral-300"
                  >
                    {target.label}
                  </span>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Proof Evidence Section */}
      {latestProof ? (
        <div className="mt-4 rounded-xl border border-white/10 bg-white/[0.03] p-4 space-y-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              {latestProof.proofType === "photo" && (
                <span className="inline-flex items-center gap-1.5 rounded-md border border-indigo-500/30 bg-indigo-500/10 px-2.5 py-1 text-xs font-medium text-indigo-300">
                  <Camera size={13} className="text-indigo-400" /> Photo Evidence
                </span>
              )}
              {latestProof.proofType === "text" && (
                <span className="inline-flex items-center gap-1.5 rounded-md border border-cyan-500/30 bg-cyan-500/10 px-2.5 py-1 text-xs font-medium text-cyan-300">
                  <FileText size={13} className="text-cyan-400" /> Written Statement
                </span>
              )}
              {latestProof.proofType === "link" && (
                <span className="inline-flex items-center gap-1.5 rounded-md border border-purple-500/30 bg-purple-500/10 px-2.5 py-1 text-xs font-medium text-purple-300">
                  <ExternalLink size={13} className="text-purple-400" /> External Link
                </span>
              )}
              {proofCount > 1 && (
                <span className="inline-flex items-center gap-1 rounded-md border border-white/10 bg-white/5 px-2 py-1 text-[11px] font-medium text-neutral-400">
                  <Layers size={11} /> Attempt #{proofCount}
                </span>
              )}
            </div>
            {latestProof.submittedAt && (
              <span className="text-[11px] text-neutral-500">
                {new Date(latestProof.submittedAt).toLocaleTimeString([], {
                  hour: "2-digit",
                  minute: "2-digit",
                })}
              </span>
            )}
          </div>

          {/* Photo Thumbnail */}
          {latestProof.photoUrl && (
            <div
              onClick={() => onInspectPhoto && onInspectPhoto(latestProof, item)}
              className="group/photo relative mt-2 max-w-sm cursor-pointer overflow-hidden rounded-lg border border-white/10 bg-black/40 transition-all hover:border-indigo-500/50"
            >
              <img
                src={latestProof.photoUrl}
                alt="Proof submission"
                className="h-44 w-full object-cover transition-transform duration-300 group-hover/photo:scale-105"
                onError={(e) => {
                  e.target.style.display = "none";
                  const fallback = e.target.parentElement.querySelector(".photo-fallback");
                  if (fallback) fallback.style.display = "flex";
                }}
              />
              <div className="photo-fallback hidden h-36 w-full flex-col items-center justify-center gap-1 bg-white/5 text-xs text-neutral-400">
                <Camera size={20} className="text-neutral-500" />
                <span>Photo attached ({latestProof.photoUrl})</span>
              </div>
              <div className="absolute inset-0 flex items-center justify-center bg-black/50 opacity-0 backdrop-blur-[2px] transition-opacity duration-200 group-hover/photo:opacity-100">
                <span className="flex items-center gap-1.5 rounded-full border border-white/20 bg-black/80 px-3 py-1.5 text-xs font-medium text-white shadow-lg">
                  <Eye size={13} /> Click to Inspect Fullscreen
                </span>
              </div>
            </div>
          )}

          {/* Written notes */}
          {latestProof.textExplanation && (
            <div className="rounded-lg border border-white/5 bg-black/40 p-3 text-xs leading-relaxed text-neutral-300">
              <span className="block font-semibold text-neutral-400 mb-1">
                Owner's Notes:
              </span>
              <p className="italic text-neutral-200">
                "{latestProof.textExplanation}"
              </p>
            </div>
          )}

          {/* External Link */}
          {latestProof.externalLink && (
            <div>
              <a
                href={latestProof.externalLink}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 text-xs font-medium text-indigo-400 hover:text-indigo-300 hover:underline"
              >
                <ExternalLink size={13} />
                {latestProof.externalLink}
              </a>
            </div>
          )}
        </div>
      ) : (
        <div className="mt-4 rounded-xl border border-white/5 bg-white/[0.01] p-4 text-xs text-neutral-500 italic">
          No verification proof submitted yet for this goal.
        </div>
      )}

      {/* Action Footer */}
      {isPending && (
        <div className="mt-5 flex flex-col gap-2 pt-4 border-t border-white/5 sm:flex-row sm:items-center sm:justify-end">
          <button
            type="button"
            onClick={() => onReject && onReject(item)}
            className="inline-flex items-center justify-center gap-1.5 rounded-xl border border-rose-500/30 bg-rose-500/10 px-4 py-2.5 text-xs font-semibold text-rose-300 transition-all hover:bg-rose-500/20 hover:border-rose-500/40 active:scale-[0.98]"
          >
            <XCircle size={15} />
            Reject Proof
          </button>
          <button
            type="button"
            onClick={() => onApprove && onApprove(item)}
            className="inline-flex items-center justify-center gap-1.5 rounded-xl border border-emerald-500/30 bg-emerald-600 px-5 py-2.5 text-xs font-semibold text-white shadow-lg shadow-emerald-950/40 transition-all hover:bg-emerald-500 active:scale-[0.98]"
          >
            <CheckCircle2 size={15} />
            Approve & Unlock
          </button>
        </div>
      )}

      {isApproved && (
        <div className="mt-4 flex items-center gap-1.5 pt-3 border-t border-white/5 text-xs font-medium text-emerald-400">
          <CheckCircle2 size={14} />
          Goal completed and targets unlocked.
        </div>
      )}

      {isRejected && (
        <div className="mt-4 flex items-center gap-1.5 pt-3 border-t border-white/5 text-xs font-medium text-rose-400">
          <AlertTriangle size={14} />
          Proof was rejected. Targets remain locked pending resubmission.
        </div>
      )}
    </div>
  );
}

