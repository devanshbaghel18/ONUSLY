import { useState, useEffect, useRef, useCallback } from "react";
import {
  X,
  ZoomIn,
  ZoomOut,
  RotateCcw,
  Camera,
  FileText,
  ExternalLink,
  Lock,
  Unlock,
  CheckCircle2,
  XCircle,
  Copy,
  Check,
  AlertCircle,
  Layers,
  Clock,
  ChevronRight,
  Maximize2,
} from "lucide-react";
import { getUniqueDisplayTargets } from "../../lib/blockedApps";

export default function EvidenceLightbox({
  isOpen,
  onClose,
  item,
  proof,
  onApprove,
  onReject,
}) {
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const [isDragging, setIsDragging] = useState(false);
  const [dragStart, setDragStart] = useState({ x: 0, y: 0 });
  const [imgLoading, setImgLoading] = useState(true);
  const [imgError, setImgError] = useState(false);
  const [copiedLink, setCopiedLink] = useState(false);

  const containerRef = useRef(null);

  // Reset zoom & pan when proof changes or modal opens
  useEffect(() => {
    if (isOpen) {
      setZoom(1);
      setPan({ x: 0, y: 0 });
      setImgLoading(true);
      setImgError(false);
      setCopiedLink(false);
      // Lock body scroll
      document.body.style.overflow = "hidden";
    }
    return () => {
      document.body.style.overflow = "";
    };
  }, [isOpen, proof?.id]);

  // Keyboard navigation
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e) => {
      if (e.key === "Escape") {
        onClose();
      } else if (e.key === "+" || e.key === "=") {
        handleZoomIn();
      } else if (e.key === "-" || e.key === "_") {
        handleZoomOut();
      } else if (e.key === "0") {
        handleResetZoom();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, zoom]);

  const handleZoomIn = () => {
    setZoom((z) => Math.min(3, +(z + 0.35).toFixed(2)));
  };

  const handleZoomOut = () => {
    setZoom((z) => {
      const next = Math.max(1, +(z - 0.35).toFixed(2));
      if (next === 1) setPan({ x: 0, y: 0 });
      return next;
    });
  };

  const handleResetZoom = () => {
    setZoom(1);
    setPan({ x: 0, y: 0 });
  };

  const handleDoubleClick = () => {
    if (zoom > 1) {
      handleResetZoom();
    } else {
      setZoom(2);
    }
  };

  // Drag-to-pan handlers
  const handleMouseDown = (e) => {
    if (zoom <= 1) return;
    setIsDragging(true);
    setDragStart({ x: e.clientX - pan.x, y: e.clientY - pan.y });
  };

  const handleMouseMove = (e) => {
    if (!isDragging || zoom <= 1) return;
    setPan({
      x: e.clientX - dragStart.x,
      y: e.clientY - dragStart.y,
    });
  };

  const handleMouseUp = () => {
    setIsDragging(false);
  };

  const handleCopyLink = (url) => {
    if (!url) return;
    navigator.clipboard.writeText(url).then(() => {
      setCopiedLink(true);
      setTimeout(() => setCopiedLink(false), 2000);
    });
  };

  if (!isOpen || !item) return null;

  const currentProof = proof || item.latestProof;
  const isPending = item.status === "proof_submitted";
  const displayTargets = getUniqueDisplayTargets(item.blockedApps, item.blockedDomains);

  return (
    <div
      className="fixed inset-0 z-50 flex flex-col bg-black/95 backdrop-blur-md select-none animate-in fade-in duration-200"
      onMouseUp={handleMouseUp}
    >
      {/* Top Header Bar */}
      <header className="flex h-16 shrink-0 items-center justify-between border-b border-white/10 px-4 sm:px-6">
        <div className="flex items-center gap-3 overflow-hidden">
          {/* Owner Identity */}
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-white/10 bg-gradient-to-br from-indigo-500/20 via-purple-500/10 to-transparent text-xs font-bold text-indigo-300">
            {(item.ownerName || item.ownerHandle || "AP").slice(0, 2).toUpperCase()}
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="truncate text-sm font-semibold text-white">
                {item.ownerName || item.ownerHandle || "Accountability Partner"}
              </span>
              <span className="hidden text-xs text-neutral-400 sm:inline">
                {item.ownerHandle ? `@${item.ownerHandle.replace(/^@/, "")}` : item.ownerEmail}
              </span>
            </div>
            <p className="truncate text-xs text-neutral-400">
              Goal: <strong className="text-neutral-200">{item.title}</strong>
            </p>
          </div>
        </div>

        {/* Center / Right controls */}
        <div className="flex items-center gap-2 sm:gap-3">
          {/* Proof type indicator */}
          {currentProof?.proofType === "photo" && (
            <span className="hidden items-center gap-1.5 rounded-lg border border-indigo-500/30 bg-indigo-500/10 px-2.5 py-1 text-xs font-medium text-indigo-300 sm:inline-flex">
              <Camera size={13} /> Photo Verification
            </span>
          )}
          {currentProof?.proofType === "text" && (
            <span className="hidden items-center gap-1.5 rounded-lg border border-cyan-500/30 bg-cyan-500/10 px-2.5 py-1 text-xs font-medium text-cyan-300 sm:inline-flex">
              <FileText size={13} /> Written Statement
            </span>
          )}
          {currentProof?.proofType === "link" && (
            <span className="hidden items-center gap-1.5 rounded-lg border border-purple-500/30 bg-purple-500/10 px-2.5 py-1 text-xs font-medium text-purple-300 sm:inline-flex">
              <ExternalLink size={13} /> External Link
            </span>
          )}

          {/* Attempt indicator */}
          {item.proofCount > 1 && (
            <span className="hidden items-center gap-1 rounded-lg border border-white/10 bg-white/5 px-2.5 py-1 text-xs font-medium text-neutral-300 md:inline-flex">
              <Layers size={12} /> Attempt #{item.proofCount}
            </span>
          )}

          {/* Zoom controls (only if photo available) */}
          {currentProof?.photoUrl && (
            <div className="flex items-center rounded-xl border border-white/10 bg-white/5 p-1 text-neutral-300">
              <button
                type="button"
                onClick={handleZoomOut}
                disabled={zoom <= 1}
                title="Zoom Out (-)"
                className="rounded-lg p-1.5 hover:bg-white/10 hover:text-white disabled:opacity-40"
              >
                <ZoomOut size={16} />
              </button>
              <button
                type="button"
                onClick={handleResetZoom}
                title="Reset Zoom (0)"
                className="px-2 text-xs font-mono font-medium hover:text-white"
              >
                {Math.round(zoom * 100)}%
              </button>
              <button
                type="button"
                onClick={handleZoomIn}
                disabled={zoom >= 3}
                title="Zoom In (+)"
                className="rounded-lg p-1.5 hover:bg-white/10 hover:text-white disabled:opacity-40"
              >
                <ZoomIn size={16} />
              </button>
              <button
                type="button"
                onClick={handleResetZoom}
                title="Reset Pan & Zoom"
                className="ml-0.5 rounded-lg p-1.5 hover:bg-white/10 hover:text-white"
              >
                <RotateCcw size={14} />
              </button>
            </div>
          )}

          {/* Close button */}
          <button
            type="button"
            onClick={onClose}
            title="Close Lightbox (Esc)"
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-white/10 bg-white/5 text-neutral-300 transition-all hover:bg-white/10 hover:text-white"
          >
            <X size={18} />
          </button>
        </div>
      </header>

      {/* Main Body: Canvas + Inspection Details Drawer */}
      <div className="flex flex-1 flex-col overflow-hidden lg:flex-row">
        {/* Canvas Area (Flex 1) */}
        <div
          ref={containerRef}
          onMouseDown={handleMouseDown}
          onMouseMove={handleMouseMove}
          onDoubleClick={handleDoubleClick}
          className={`relative flex flex-1 items-center justify-center overflow-hidden bg-black/60 p-4 ${
            zoom > 1
              ? isDragging
                ? "cursor-grabbing"
                : "cursor-grab"
              : "cursor-zoom-in"
          }`}
        >
          {currentProof?.photoUrl ? (
            <div className="relative flex max-h-full max-w-full items-center justify-center">
              {imgLoading && (
                <div className="absolute inset-0 flex flex-col items-center justify-center gap-2">
                  <div className="h-8 w-8 animate-spin rounded-full border-2 border-indigo-500 border-t-transparent" />
                  <span className="text-xs text-neutral-400">Loading evidence in high-resolution...</span>
                </div>
              )}

              {imgError ? (
                <div className="flex flex-col items-center justify-center gap-2 rounded-2xl border border-red-500/20 bg-red-500/5 p-8 text-center text-red-300">
                  <AlertCircle size={32} className="text-red-400" />
                  <p className="text-sm font-semibold">Failed to load photo</p>
                  <p className="text-xs text-neutral-400">{currentProof.photoUrl}</p>
                </div>
              ) : (
                <img
                  src={currentProof.photoUrl}
                  alt="Proof Evidence"
                  draggable={false}
                  onLoad={() => setImgLoading(false)}
                  onError={() => {
                    setImgLoading(false);
                    setImgError(true);
                  }}
                  style={{
                    transform: `scale(${zoom}) translate(${pan.x / zoom}px, ${pan.y / zoom}px)`,
                    transition: isDragging ? "none" : "transform 0.15s ease-out",
                  }}
                  className={`max-h-[80vh] max-w-full rounded-xl object-contain shadow-2xl transition-opacity duration-200 ${
                    imgLoading ? "opacity-0" : "opacity-100"
                  }`}
                />
              )}
            </div>
          ) : (
            <div className="flex flex-col items-center justify-center gap-3 p-8 text-center text-neutral-400">
              <FileText size={48} className="text-neutral-500" />
              <p className="text-sm font-medium text-white">Non-visual proof submission</p>
              <p className="max-w-md text-xs text-neutral-400">
                This verification proof does not include an attached photo. Please review the written statement and link on the inspection panel.
              </p>
            </div>
          )}

          {/* Quick instructions floating badge */}
          {currentProof?.photoUrl && zoom === 1 && (
            <div className="pointer-events-none absolute bottom-4 left-1/2 -translate-x-1/2 rounded-full border border-white/10 bg-black/70 px-3 py-1 text-[11px] text-neutral-400 backdrop-blur-sm">
              Double-click or use controls to zoom • Esc to exit
            </div>
          )}
        </div>

        {/* Inspection Details Panel (Right Sidebar on Desktop, Bottom Sheet on Mobile) */}
        <aside className="w-full shrink-0 border-t border-white/10 bg-[#0e0e12] p-6 lg:w-96 lg:border-l lg:border-t-0 overflow-y-auto">
          <div className="space-y-6">
            {/* Header info */}
            <div>
              <span className="text-xs font-semibold uppercase tracking-wider text-neutral-500">
                Evidence Details
              </span>
              <h2 className="mt-1 text-lg font-bold text-white tracking-tight">
                {item.title}
              </h2>
              {item.description && (
                <p className="mt-1 text-xs text-neutral-400 leading-relaxed">
                  {item.description}
                </p>
              )}
            </div>

            {/* Blocked Targets to Unlock */}
            {displayTargets.length > 0 && (
              <div className="rounded-xl border border-white/10 bg-white/[0.02] p-4">
                <div className="flex items-center gap-2 text-xs font-semibold text-neutral-300 mb-2">
                  <Lock size={13} className="text-amber-400" />
                  <span>Targets Locked Pending Decision:</span>
                </div>
                <div className="flex flex-wrap gap-1.5">
                  {displayTargets.map((target, idx) => (
                    <span
                      key={idx}
                      className="rounded-md border border-white/10 bg-white/5 px-2 py-0.5 text-xs text-neutral-300"
                    >
                      {target.label}
                    </span>
                  ))}
                </div>
              </div>
            )}

            {/* Submission Time */}
            {currentProof?.submittedAt && (
              <div className="flex items-center gap-2 text-xs text-neutral-400">
                <Clock size={13} />
                <span>
                  Submitted on{" "}
                  <strong className="text-neutral-200">
                    {new Date(currentProof.submittedAt).toLocaleString()}
                  </strong>
                </span>
              </div>
            )}

            {/* Written Explanation */}
            {currentProof?.textExplanation ? (
              <div className="rounded-xl border border-white/10 bg-white/[0.03] p-4 space-y-1.5">
                <span className="text-xs font-semibold text-neutral-400 flex items-center gap-1.5">
                  <FileText size={13} className="text-cyan-400" />
                  Partner's Written Note:
                </span>
                <p className="text-xs italic leading-relaxed text-neutral-200">
                  "{currentProof.textExplanation}"
                </p>
              </div>
            ) : (
              <div className="rounded-xl border border-white/5 bg-white/[0.01] p-3 text-xs text-neutral-500 italic">
                No written note accompanied this submission.
              </div>
            )}

            {/* External Resource Link */}
            {currentProof?.externalLink && (
              <div className="rounded-xl border border-white/10 bg-white/[0.03] p-4 space-y-2">
                <span className="text-xs font-semibold text-neutral-400 flex items-center gap-1.5">
                  <ExternalLink size={13} className="text-purple-400" />
                  Referenced Link:
                </span>
                <div className="flex items-center justify-between gap-2 rounded-lg border border-white/10 bg-black/40 px-3 py-2 text-xs">
                  <a
                    href={currentProof.externalLink}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="truncate text-indigo-400 hover:text-indigo-300 hover:underline"
                  >
                    {currentProof.externalLink}
                  </a>
                  <button
                    type="button"
                    onClick={() => handleCopyLink(currentProof.externalLink)}
                    className="shrink-0 text-neutral-400 hover:text-white"
                    title="Copy URL"
                  >
                    {copiedLink ? (
                      <Check size={14} className="text-emerald-400" />
                    ) : (
                      <Copy size={14} />
                    )}
                  </button>
                </div>
              </div>
            )}

            {/* Decision Controls inside Lightbox */}
            {isPending && (
              <div className="space-y-3 pt-4 border-t border-white/10">
                <span className="text-xs font-semibold uppercase tracking-wider text-neutral-400">
                  Make Decision
                </span>
                <div className="flex flex-col gap-2">
                  <button
                    type="button"
                    onClick={() => {
                      onClose();
                      onApprove && onApprove(item);
                    }}
                    className="flex w-full items-center justify-center gap-2 rounded-xl border border-emerald-500/30 bg-emerald-600 px-4 py-3 text-xs font-semibold text-white shadow-lg shadow-emerald-950/50 transition-all hover:bg-emerald-500 active:scale-[0.98]"
                  >
                    <CheckCircle2 size={16} />
                    Approve & Unlock Targets
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      onClose();
                      onReject && onReject(item);
                    }}
                    className="flex w-full items-center justify-center gap-2 rounded-xl border border-rose-500/30 bg-rose-500/10 px-4 py-2.5 text-xs font-semibold text-rose-300 transition-all hover:bg-rose-500/20 active:scale-[0.98]"
                  >
                    <XCircle size={16} />
                    Reject with Feedback
                  </button>
                </div>
              </div>
            )}

            {!isPending && (
              <div className="pt-4 border-t border-white/10 text-xs text-neutral-400">
                Decision has already been recorded for this goal.
              </div>
            )}
          </div>
        </aside>
      </div>
    </div>
  );
}
