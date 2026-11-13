import { useState, useEffect, useRef } from "react";
import {
  X,
  ZoomIn,
  ZoomOut,
  RotateCcw,
  Camera,
  FileText,
  ExternalLink,
  Lock,
  CheckCircle2,
  XCircle,
  Copy,
  Check,
  AlertCircle,
  Layers,
  Clock,
  ChevronLeft,
  ChevronRight,
  Download,
  File,
} from "lucide-react";
import { getUniqueDisplayTargets } from "../../lib/blockedApps";

/**
 * Extracts all attached files from a proof submission.
 * Supports:
 *  1. Structured files payload (multi-document / photo uploads)
 *  2. Legacy textExplanation manifest parsing with photoUrl
 *  3. Single photoUrl fallback
 */
function extractProofFiles(currentProof) {
  if (!currentProof) return [];

  // 1. Structured files array passed from API / WebSocket
  if (Array.isArray(currentProof.files) && currentProof.files.length > 0) {
    return currentProof.files.map((f, idx) => ({
      id: f.id || `file-${idx}`,
      name: f.name || `Attachment ${idx + 1}`,
      size: f.size || 0,
      formattedSize:
        f.formattedSize ||
        (f.size ? `${Math.round(f.size / 1024)} KB` : ""),
      type: f.type || "",
      dataUrl: f.dataUrl || "",
      isImage: Boolean(
        f.isImage ||
          (f.dataUrl && f.dataUrl.startsWith("data:image/")) ||
          (f.type && f.type.startsWith("image/"))
      ),
      isPdf: Boolean(
        f.isPdf ||
          (f.dataUrl && f.dataUrl.startsWith("data:application/pdf")) ||
          (f.name && f.name.toLowerCase().endsWith(".pdf"))
      ),
    }));
  }

  // 2. Parse from textExplanation if notes contain [Attached Proof Documents (N)] manifest (legacy)
  const filesFromManifest = [];
  if (
    currentProof.textExplanation &&
    currentProof.textExplanation.includes("[Attached Proof Documents")
  ) {
    const lines = currentProof.textExplanation.split("\n");
    let inManifest = false;
    for (const line of lines) {
      if (line.includes("[Attached Proof Documents")) {
        inManifest = true;
        continue;
      }
      if (inManifest) {
        const match = line.trim().match(/^\d+\.\s+(.*?)(?:\s+\((.*?)\))?$/);
        if (match) {
          const fileName = match[1];
          const fileSize = match[2] || "";
          const isPdf = fileName.toLowerCase().endsWith(".pdf");
          const isImage = /\.(jpe?g|png|webp|gif|bmp|svg)$/i.test(fileName);
          filesFromManifest.push({
            id: `manifest-${filesFromManifest.length}`,
            name: fileName,
            formattedSize: fileSize,
            isImage,
            isPdf,
            dataUrl: "", // Populated below if photoUrl matches
          });
        }
      }
    }
  }

  if (filesFromManifest.length > 0) {
    if (currentProof.photoUrl) {
      const firstImgIdx = filesFromManifest.findIndex((f) => f.isImage);
      if (firstImgIdx !== -1) {
        filesFromManifest[firstImgIdx].dataUrl = currentProof.photoUrl;
      } else {
        filesFromManifest.unshift({
          id: "primary-photo",
          name: "Photo Evidence",
          dataUrl: currentProof.photoUrl,
          isImage: true,
          isPdf: false,
        });
      }
    }
    return filesFromManifest;
  }

  // 3. Fallback: single photoUrl
  if (currentProof.photoUrl) {
    const isPdf =
      currentProof.photoUrl.startsWith("data:application/pdf") ||
      currentProof.photoUrl.toLowerCase().endsWith(".pdf");
    return [
      {
        id: "single-proof",
        name: isPdf ? "Document Evidence.pdf" : "Photo Evidence",
        dataUrl: currentProof.photoUrl,
        isImage: !isPdf,
        isPdf,
      },
    ];
  }

  return [];
}

export default function EvidenceLightbox({
  isOpen,
  onClose,
  item,
  proof,
  initialFileIndex = 0,
  onApprove,
  onReject,
}) {
  const [activeFileIndex, setActiveFileIndex] = useState(0);
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const [isDragging, setIsDragging] = useState(false);
  const [dragStart, setDragStart] = useState({ x: 0, y: 0 });
  const [imgLoading, setImgLoading] = useState(true);
  const [imgError, setImgError] = useState(false);
  const [copiedLink, setCopiedLink] = useState(false);

  const containerRef = useRef(null);

  const currentProof = proof || item?.latestProof;
  const attachedFiles = extractProofFiles(currentProof);
  const activeFile =
    attachedFiles[activeFileIndex] || attachedFiles[0] || null;

  // Reset zoom, pan, & active index when proof changes or modal opens
  useEffect(() => {
    if (isOpen) {
      setActiveFileIndex(
        initialFileIndex >= 0 && initialFileIndex < attachedFiles.length
          ? initialFileIndex
          : 0
      );
      setZoom(1);
      setPan({ x: 0, y: 0 });
      setImgLoading(true);
      setImgError(false);
      setCopiedLink(false);
      document.body.style.overflow = "hidden";
    }
    return () => {
      document.body.style.overflow = "";
    };
  }, [isOpen, currentProof?.id, initialFileIndex, attachedFiles.length]);

  // Reset pan and zoom whenever active file changes
  useEffect(() => {
    setZoom(1);
    setPan({ x: 0, y: 0 });
    setImgLoading(true);
    setImgError(false);
  }, [activeFileIndex]);

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
      } else if (e.key === "ArrowLeft") {
        handlePrevFile();
      } else if (e.key === "ArrowRight") {
        handleNextFile();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, zoom, activeFileIndex, attachedFiles.length]);

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

  const handlePrevFile = () => {
    if (attachedFiles.length <= 1) return;
    setActiveFileIndex((prev) =>
      prev > 0 ? prev - 1 : attachedFiles.length - 1
    );
  };

  const handleNextFile = () => {
    if (attachedFiles.length <= 1) return;
    setActiveFileIndex((prev) =>
      prev < attachedFiles.length - 1 ? prev + 1 : 0
    );
  };

  // Drag-to-pan handlers for zoomed images
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

  const isPending = item.status === "proof_submitted";
  const displayTargets = getUniqueDisplayTargets(
    item.blockedApps,
    item.blockedDomains
  );

  return (
    <div
      className="fixed inset-0 z-50 flex flex-col bg-black/95 backdrop-blur-md select-none animate-in fade-in duration-200"
      onMouseUp={handleMouseUp}
    >
      {/* ============================================================
          TOP HEADER BAR
      ============================================================ */}
      <header className="flex h-16 shrink-0 items-center justify-between border-b border-white/10 px-4 sm:px-6">
        <div className="flex items-center gap-3 overflow-hidden">
          {/* Owner Avatar */}
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-white/10 bg-gradient-to-br from-indigo-500/20 via-purple-500/10 to-transparent text-xs font-bold text-indigo-300">
            {(item.ownerName || item.ownerHandle || "AP")
              .slice(0, 2)
              .toUpperCase()}
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="truncate text-sm font-semibold text-white">
                {item.ownerName ||
                  item.ownerHandle ||
                  "Accountability Partner"}
              </span>
              <span className="hidden text-xs text-neutral-400 sm:inline">
                {item.ownerHandle
                  ? `@${item.ownerHandle.replace(/^@/, "")}`
                  : item.ownerEmail}
              </span>
            </div>
            <p className="truncate text-xs text-neutral-400">
              Goal: <strong className="text-neutral-200">{item.title}</strong>
            </p>
          </div>
        </div>

        {/* Center / Right controls */}
        <div className="flex items-center gap-2 sm:gap-3">
          {/* File counter badge */}
          {attachedFiles.length > 0 && (
            <span className="inline-flex items-center gap-1.5 rounded-lg border border-white/10 bg-white/5 px-2.5 py-1 text-xs font-medium text-neutral-300">
              <Layers size={13} className="text-indigo-400" />
              <span>
                {activeFileIndex + 1} of {attachedFiles.length}
              </span>
            </span>
          )}

          {/* Zoom controls (active only if current file is an image with dataUrl) */}
          {activeFile?.isImage && activeFile?.dataUrl && (
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

      {/* ============================================================
          MAIN BODY: DOCUMENT CAROUSEL & PREVIEW + INSPECTION PANEL
      ============================================================ */}
      <div className="flex flex-1 flex-col overflow-hidden lg:flex-row">
        {/* Left / Center Area: Document Tabs + Main Preview Canvas */}
        <div className="relative flex flex-1 flex-col overflow-hidden bg-black/60">
          {/* Multi-Document Tab Selector Bar (if > 1 file attached) */}
          {attachedFiles.length > 1 && (
            <div className="flex shrink-0 items-center gap-2 overflow-x-auto border-b border-white/10 bg-[#0d0d11] px-4 py-2.5">
              <span className="text-[11px] font-bold uppercase tracking-wider text-neutral-500 mr-1 shrink-0">
                Attachments:
              </span>
              {attachedFiles.map((f, idx) => {
                const isActive = idx === activeFileIndex;
                return (
                  <button
                    key={f.id}
                    type="button"
                    onClick={() => setActiveFileIndex(idx)}
                    className={`inline-flex shrink-0 items-center gap-2 rounded-xl px-3 py-1.5 text-xs font-medium transition-all ${
                      isActive
                        ? "border border-indigo-500/50 bg-indigo-500/20 text-white shadow-md shadow-indigo-950/40"
                        : "border border-white/10 bg-white/5 text-neutral-400 hover:border-white/20 hover:text-neutral-200"
                    }`}
                  >
                    {f.isPdf ? (
                      <FileText size={14} className="text-rose-400 shrink-0" />
                    ) : f.isImage ? (
                      <Camera size={14} className="text-indigo-400 shrink-0" />
                    ) : (
                      <File size={14} className="text-cyan-400 shrink-0" />
                    )}
                    <span className="max-w-[140px] truncate">{f.name}</span>
                    {f.formattedSize && (
                      <span className="text-[10px] text-neutral-400">
                        {f.formattedSize}
                      </span>
                    )}
                  </button>
                );
              })}
            </div>
          )}

          {/* Canvas Display Area */}
          <div
            ref={containerRef}
            onMouseDown={handleMouseDown}
            onMouseMove={handleMouseMove}
            onDoubleClick={activeFile?.isImage ? handleDoubleClick : undefined}
            className={`relative flex flex-1 items-center justify-center overflow-hidden p-4 sm:p-6 ${
              activeFile?.isImage && zoom > 1
                ? isDragging
                  ? "cursor-grabbing"
                  : "cursor-grab"
                : activeFile?.isImage
                ? "cursor-zoom-in"
                : "cursor-default"
            }`}
          >
            {/* Previous File Arrow (<) */}
            {attachedFiles.length > 1 && (
              <button
                type="button"
                onClick={handlePrevFile}
                title="Previous Document (Left Arrow)"
                className="absolute left-4 top-1/2 -translate-y-1/2 z-20 flex h-10 w-10 items-center justify-center rounded-full border border-white/15 bg-black/70 text-white shadow-xl backdrop-blur-md transition-all hover:scale-110 hover:bg-black/90 active:scale-95"
              >
                <ChevronLeft size={22} />
              </button>
            )}

            {/* Next File Arrow (>) */}
            {attachedFiles.length > 1 && (
              <button
                type="button"
                onClick={handleNextFile}
                title="Next Document (Right Arrow)"
                className="absolute right-4 top-1/2 -translate-y-1/2 z-20 flex h-10 w-10 items-center justify-center rounded-full border border-white/15 bg-black/70 text-white shadow-xl backdrop-blur-md transition-all hover:scale-110 hover:bg-black/90 active:scale-95"
              >
                <ChevronRight size={22} />
              </button>
            )}

            {/* Case 1: Active file is an IMAGE */}
            {activeFile?.isImage && activeFile?.dataUrl ? (
              <div className="relative flex max-h-full max-w-full items-center justify-center">
                {imgLoading && (
                  <div className="absolute inset-0 flex flex-col items-center justify-center gap-2">
                    <div className="h-8 w-8 animate-spin rounded-full border-2 border-indigo-500 border-t-transparent" />
                    <span className="text-xs text-neutral-400">
                      Loading high-resolution image...
                    </span>
                  </div>
                )}

                {imgError ? (
                  <div className="flex flex-col items-center justify-center gap-2 rounded-2xl border border-red-500/20 bg-red-500/5 p-8 text-center text-red-300">
                    <AlertCircle size={32} className="text-red-400" />
                    <p className="text-sm font-semibold">Failed to load photo</p>
                    <p className="text-xs text-neutral-400">{activeFile.name}</p>
                  </div>
                ) : (
                  <img
                    src={activeFile.dataUrl}
                    alt={activeFile.name || "Proof Evidence"}
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
                    className={`max-h-[75vh] max-w-full rounded-xl object-contain shadow-2xl transition-opacity duration-200 ${
                      imgLoading ? "opacity-0" : "opacity-100"
                    }`}
                  />
                )}
              </div>
            ) : null}

            {/* Case 2: Active file is a PDF */}
            {activeFile?.isPdf && (
              <div className="flex h-full w-full max-w-4xl flex-col rounded-2xl border border-white/10 bg-[#15151c] shadow-2xl overflow-hidden">
                {/* PDF Toolbar Header */}
                <div className="flex items-center justify-between border-b border-white/10 bg-white/5 px-4 py-2.5">
                  <div className="flex items-center gap-2.5 min-w-0">
                    <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-rose-500/20 text-rose-400 border border-rose-500/30">
                      <FileText size={16} />
                    </div>
                    <div className="min-w-0">
                      <h4 className="text-xs font-semibold text-white truncate">
                        {activeFile.name}
                      </h4>
                      {activeFile.formattedSize && (
                        <span className="text-[10px] text-neutral-400">
                          {activeFile.formattedSize} • Adobe PDF Document
                        </span>
                      )}
                    </div>
                  </div>

                  {activeFile.dataUrl && (
                    <div className="flex items-center gap-2 shrink-0">
                      <a
                        href={activeFile.dataUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex items-center gap-1.5 rounded-lg border border-white/10 bg-white/5 px-3 py-1.5 text-xs font-medium text-neutral-300 hover:bg-white/10 hover:text-white transition"
                      >
                        <ExternalLink size={13} />
                        <span>Open in New Tab</span>
                      </a>
                      <a
                        href={activeFile.dataUrl}
                        download={activeFile.name || "evidence.pdf"}
                        className="inline-flex items-center gap-1.5 rounded-lg bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-indigo-500 transition shadow"
                      >
                        <Download size={13} />
                        <span>Download</span>
                      </a>
                    </div>
                  )}
                </div>

                {/* PDF Embedded Frame */}
                {activeFile.dataUrl ? (
                  <iframe
                    src={activeFile.dataUrl}
                    title={activeFile.name}
                    className="h-[68vh] w-full border-0 bg-white"
                  />
                ) : (
                  <div className="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-center text-neutral-400">
                    <FileText size={48} className="text-rose-400/80" />
                    <h5 className="text-sm font-semibold text-white">
                      {activeFile.name}
                    </h5>
                    <p className="max-w-md text-xs text-neutral-400 leading-relaxed">
                      This document was registered in the proof manifest ({activeFile.formattedSize || "PDF"}).
                      Submit a new verification proof to view embedded PDF previews directly.
                    </p>
                  </div>
                )}
              </div>
            )}

            {/* Case 3: Other document without inline viewer */}
            {activeFile && !activeFile.isImage && !activeFile.isPdf && (
              <div className="flex flex-col items-center justify-center gap-4 rounded-2xl border border-white/10 bg-[#15151c] p-8 text-center max-w-md shadow-2xl">
                <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-indigo-500/10 text-indigo-400 border border-indigo-500/20">
                  <File size={32} />
                </div>
                <div>
                  <h4 className="text-base font-semibold text-white truncate max-w-xs">
                    {activeFile.name}
                  </h4>
                  <p className="text-xs text-neutral-400 mt-1">
                    {activeFile.formattedSize || "Attached Document"}
                  </p>
                </div>
                {activeFile.dataUrl && (
                  <a
                    href={activeFile.dataUrl}
                    download={activeFile.name}
                    className="inline-flex items-center gap-2 rounded-xl bg-indigo-600 px-4 py-2.5 text-xs font-semibold text-white hover:bg-indigo-500 transition shadow-lg"
                  >
                    <Download size={14} />
                    <span>Download Attached File</span>
                  </a>
                )}
              </div>
            )}

            {/* Case 4: No files at all (written proof / link only) */}
            {attachedFiles.length === 0 && (
              <div className="flex flex-col items-center justify-center gap-3 p-8 text-center text-neutral-400">
                <FileText size={48} className="text-neutral-500" />
                <p className="text-sm font-medium text-white">
                  Non-visual proof submission
                </p>
                <p className="max-w-md text-xs text-neutral-400">
                  This verification proof does not include attached files.
                  Please review the written statement and link on the inspection panel.
                </p>
              </div>
            )}

            {/* Quick instructions floating badge */}
            {activeFile?.isImage && zoom === 1 && (
              <div className="pointer-events-none absolute bottom-4 left-1/2 -translate-x-1/2 rounded-full border border-white/10 bg-black/70 px-3 py-1 text-[11px] text-neutral-400 backdrop-blur-sm">
                Double-click to zoom • Use arrows to switch documents • Esc to exit
              </div>
            )}
          </div>
        </div>

        {/* ============================================================
            RIGHT INSPECTION PANEL
        ============================================================ */}
        <aside className="w-full shrink-0 border-t border-white/10 bg-[#0e0e12] p-6 lg:w-96 lg:border-l lg:border-t-0 overflow-y-auto max-h-[90vh]">
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

            {/* Attached Files Manifest Summary */}
            {attachedFiles.length > 0 && (
              <div className="rounded-xl border border-white/10 bg-white/[0.02] p-4 space-y-2.5">
                <div className="flex items-center justify-between text-xs font-semibold text-neutral-300">
                  <span className="flex items-center gap-1.5">
                    <Layers size={13} className="text-indigo-400" />
                    Attached Proof Files ({attachedFiles.length}):
                  </span>
                </div>
                <div className="space-y-1.5">
                  {attachedFiles.map((f, idx) => (
                    <button
                      key={f.id}
                      type="button"
                      onClick={() => setActiveFileIndex(idx)}
                      className={`flex w-full items-center justify-between rounded-lg px-2.5 py-1.5 text-left text-xs transition ${
                        idx === activeFileIndex
                          ? "bg-indigo-500/20 text-white font-semibold border border-indigo-500/40"
                          : "text-neutral-300 hover:bg-white/5 hover:text-white"
                      }`}
                    >
                      <div className="flex items-center gap-2 min-w-0">
                        {f.isPdf ? (
                          <FileText size={13} className="text-rose-400 shrink-0" />
                        ) : f.isImage ? (
                          <Camera size={13} className="text-indigo-400 shrink-0" />
                        ) : (
                          <File size={13} className="text-cyan-400 shrink-0" />
                        )}
                        <span className="truncate">{f.name}</span>
                      </div>
                      {f.formattedSize && (
                        <span className="text-[10px] text-neutral-500 shrink-0 ml-2">
                          {f.formattedSize}
                        </span>
                      )}
                    </button>
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
                <p className="text-xs italic leading-relaxed text-neutral-200 whitespace-pre-wrap">
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
