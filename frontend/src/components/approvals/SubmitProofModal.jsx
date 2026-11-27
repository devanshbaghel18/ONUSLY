import { useState, useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import {
  X,
  UploadCloud,
  Camera,
  FileText,
  Link as LinkIcon,
  AlertCircle,
  Loader2,
  Trash2,
  Users,
  Plus,
  Send,
} from "lucide-react";
import { getUser } from "../../lib/auth";
import { submitProof, uploadProofFiles, resolveApiUrl } from "../../lib/api";
import { playProofSubmittedSound } from "../../lib/sound";

function generateMsgId(prefix = "msg") {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).substring(2, 7)}`;
}

function formatBytes(bytes) {
  if (!bytes || bytes <= 0) return "0 B";
  if (bytes > 1024 * 1024) {
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }
  return `${Math.round(bytes / 1024)} KB`;
}

// Compress single image via HTML canvas for lightweight preview; non-images upload directly via multipart FormData
function processImageFile(file, maxDimension = 1400, quality = 0.82) {
  return new Promise((resolve) => {
    if (!file.type.startsWith("image/")) {
      resolve("");
      return;
    }

    const reader = new FileReader();
    reader.onload = (e) => {
      const img = new Image();
      img.onload = () => {
        let width = img.width;
        let height = img.height;

        if (width > maxDimension || height > maxDimension) {
          if (width > height) {
            height = Math.round((height * maxDimension) / width);
            width = maxDimension;
          } else {
            width = Math.round((width * maxDimension) / height);
            height = maxDimension;
          }
        }

        const canvas = document.createElement("canvas");
        canvas.width = width;
        canvas.height = height;
        const ctx = canvas.getContext("2d");
        ctx.drawImage(img, 0, 0, width, height);

        const dataUrl = canvas.toDataURL("image/jpeg", quality);
        resolve(dataUrl);
      };
      img.onerror = () => resolve(e.target.result);
      img.src = e.target.result;
    };
    reader.onerror = () => resolve("");
    reader.readAsDataURL(file);
  });
}

export default function SubmitProofModal({
  isOpen,
  onClose,
  goal,
  onSuccess,
  send,
}) {
  const [attachedFiles, setAttachedFiles] = useState([]);
  const [textExplanation, setTextExplanation] = useState("");
  const [externalLink, setExternalLink] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  const fileInputRef = useRef(null);

  useEffect(() => {
    if (!isOpen) return;
    let cancelled = false;
    Promise.resolve().then(() => {
      if (cancelled) return;
      setAttachedFiles([]);
      setTextExplanation("");
      setExternalLink("");
      setError("");
      setSubmitting(false);
    });
    return () => {
      cancelled = true;
    };
  }, [isOpen]);

  // Escape key closes modal
  useEffect(() => {
    if (!isOpen || submitting) return;
    const handleKeyDown = (e) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, submitting, onClose]);

  if (!isOpen || !goal) return null;

  const hasAssignedApprover = Boolean(goal.approverEmail || goal.approverId);
  const approverDisplay =
    goal.approverEmail ||
    (goal.approvalType === "community" ? "Community" : "Assigned Partner");

  const handleFilesChange = async (e) => {
    const selected = Array.from(e.target.files || []);
    if (selected.length === 0) return;

    setError("");
    try {
      const processedList = await Promise.all(
        selected.map(async (file) => {
          const isImage = file.type.startsWith("image/");
          const isPdf = file.type === "application/pdf" || file.name.toLowerCase().endsWith(".pdf");
          const dataUrl = await processImageFile(file);

          return {
            id: `file-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
            name: file.name,
            size: file.size,
            formattedSize: formatBytes(file.size),
            type: file.type,
            isImage,
            isPdf,
            dataUrl,
            rawFile: file,
          };
        })
      );

      // Append new files without replacing already added ones
      setAttachedFiles((prev) => [...prev, ...processedList]);
    } catch (err) {
      console.error("Failed to process attached files:", err);
      setError("Failed to read one or more files. Please select valid images or documents.");
    } finally {
      if (fileInputRef.current) fileInputRef.current.value = "";
    }
  };

  const handleRemoveFile = (idToRemove) => {
    setAttachedFiles((prev) => prev.filter((f) => f.id !== idToRemove));
  };

  const handleClearAllFiles = () => {
    setAttachedFiles([]);
    if (fileInputRef.current) fileInputRef.current.value = "";
  };

  const handleSubmit = async (e) => {
    e?.preventDefault();
    if (submitting) return;

    const trimmedNotes = textExplanation.trim();
    const trimmedLink = externalLink.trim();

    if (attachedFiles.length === 0 && !trimmedNotes && !trimmedLink) {
      setError("Please attach at least one photo/document, enter an explanation, or provide a link.");
      return;
    }

    setSubmitting(true);
    setError("");

    try {
      const currentUser = getUser();
      const goalId = goal.id;

      // 1. Upload raw files to server if any raw files exist (match deterministically by index)
      let finalFiles = [...attachedFiles];
      const rawEntries = attachedFiles
        .map((f, idx) => ({ f, idx }))
        .filter((entry) => Boolean(entry.f.rawFile));

      if (rawEntries.length > 0) {
        const uploadedResults = await uploadProofFiles(
          rawEntries.map((entry) => entry.f.rawFile),
          { goalId, scope: "proofs" }
        );
        if (!Array.isArray(uploadedResults) || uploadedResults.length !== rawEntries.length) {
          throw new Error("Upload incomplete: server did not confirm all attached files.");
        }
        const byIndex = new Map();
        rawEntries.forEach((entry, i) => {
          const uploaded = uploadedResults[i];
          if (!uploaded || !uploaded.url) {
            throw new Error(`Upload failed for "${entry.f.name}".`);
          }
          const fullUrl = resolveApiUrl(uploaded.url);
          byIndex.set(entry.idx, {
            ...entry.f,
            url: fullUrl,
            dataUrl: fullUrl,
            type: uploaded.type || entry.f.type,
            isImage: typeof uploaded.isImage === "boolean" ? uploaded.isImage : entry.f.isImage,
            isPdf: typeof uploaded.isPdf === "boolean" ? uploaded.isPdf : entry.f.isPdf,
          });
        });
        finalFiles = attachedFiles.map((f, idx) => byIndex.get(idx) || f);
      }

      // 2. Prepare structured files payload
      const filesPayload = finalFiles.map((f) => ({
        id: f.id,
        name: f.name,
        size: f.size,
        formattedSize: f.formattedSize,
        type: f.type,
        dataUrl: f.dataUrl,
        url: f.url || f.dataUrl,
        isImage: f.isImage,
        isPdf: f.isPdf,
      }));

      // 3. Set primary photoUrl (first image or document) without stitching
      let finalPhotoUrl = "";
      const imageFiles = finalFiles.filter((f) => f.isImage && f.dataUrl);
      const docFiles = finalFiles.filter((f) => !f.isImage && f.dataUrl);

      if (imageFiles.length > 0) {
        finalPhotoUrl = imageFiles[0].dataUrl;
      } else if (docFiles.length > 0) {
        finalPhotoUrl = docFiles[0].dataUrl;
      }

      // 4. Determine proofType
      let proofType = "text";
      if (imageFiles.length > 0) proofType = "photo";
      else if (docFiles.length > 0) proofType = "document";
      else if (trimmedLink) proofType = "link";

      // 5. Append attached documents manifest to notes if multiple files exist
      let finalExplanation = trimmedNotes;
      if (finalFiles.length > 0) {
        const fileManifest = finalFiles
          .map((f, i) => `  ${i + 1}. ${f.name} (${f.formattedSize})`)
          .join("\n");
        const suffix = `\n\n[Attached Proof Documents (${finalFiles.length})]:\n${fileManifest}`;
        finalExplanation = trimmedNotes ? `${trimmedNotes}${suffix}` : `[Attached Proof Documents (${finalFiles.length})]:\n${fileManifest}`;
      }

      // 6. Submit proof to backend REST API
      const result = await submitProof(goalId, {
        proofType,
        photoUrl: finalPhotoUrl,
        textExplanation: finalExplanation,
        externalLink: trimmedLink,
        files: filesPayload,
      });

      // Send real-time chat notification to partner over WebSocket
      try {
        const senderEmail = currentUser?.email || "";
        const senderHandle = currentUser?.handle || "";
        const senderName =
          currentUser?.name || (senderHandle ? `@${senderHandle}` : "You");
        const msgId = generateMsgId("proof-msg");

        const sendWS = (msg) => {
          if (typeof send === "function") send(msg);
        };

        if (hasAssignedApprover) {
          sendWS({
            type: "chat.message",
            payload: {
              id: msgId,
              recipientEmail: goal.approverEmail || "",
              senderEmail,
              senderHandle,
              senderName,
              text: `Submitted verification proof for goal: "${goal.title}" (${finalFiles.length} file(s) attached)`,
              isProof: true,
              goalId,
              goalTitle: goal.title,
              ownerId: currentUser?.id || "",
              proofType,
              photoUrl: finalPhotoUrl,
              textExplanation: finalExplanation,
              fileCount: finalFiles.length,
              files: filesPayload,
            },
          });
        }
      } catch (wsErr) {
        console.warn("Background chat notification skipped:", wsErr.message);
      }

      playProofSubmittedSound();

      if (onSuccess) {
        onSuccess(result);
      }

      onClose();
    } catch (err) {
      console.error("Failed to submit proof:", err);
      setError(err?.message || "Failed to submit verification proof. Please try again.");
    } finally {
      setSubmitting(false);
    }
  };

  return createPortal(
    <div
      role="dialog"
      aria-modal="true"
      className="fixed inset-0 z-[9999] flex items-center justify-center modal-backdrop p-4"
      onClick={(e) => {
        if (e.target === e.currentTarget && !submitting) onClose();
      }}
    >
      <div className="relative w-full max-w-lg rounded-lg border border-[#2b3542] bg-[#12161c] p-6 shadow-[0_25px_60px_rgba(0,0,0,0.95)] max-h-[90vh] overflow-y-auto animate-scale-in">
        {/* Close Button */}
        <button
          type="button"
          onClick={onClose}
          disabled={submitting}
          className="absolute right-4 top-4 rounded border border-[#26313d] bg-[#161c23] p-1.5 text-neutral-400 hover:border-[#384654] hover:text-white transition disabled:opacity-40"
        >
          <X size={16} />
        </button>

        {/* Modal Header */}
        <div className="flex items-center gap-3 border-b border-[#26313d] pb-4">
          <div className="flex h-9 w-9 items-center justify-center rounded border border-[#2d3744] bg-[#182028] text-[#ff7843] shadow-sm">
            <Camera size={18} />
          </div>
          <div className="min-w-0 pr-6">
            <h2 className="text-base font-bold text-white tracking-tight">
              Submit Proof for Verification
            </h2>
            <p className="truncate text-xs text-[#8c9ba8]">
              Attach evidence of your completed work for "{goal.title}"
            </p>
          </div>
        </div>

        {/* Error Alert */}
        {error && (
          <div className="mt-4 flex items-center gap-2 rounded border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-300">
            <AlertCircle size={15} className="shrink-0 text-red-400" />
            <span>{error}</span>
          </div>
        )}

        <form onSubmit={handleSubmit} className="mt-5 space-y-4">
          {/* Multi-File Upload / Documents Picker from PC */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="font-mono text-[10px] uppercase font-bold tracking-wider text-neutral-400 flex items-center gap-1.5">
                <span>Proof Documents & Photos</span>
                {attachedFiles.length > 0 && (
                  <span className="rounded border border-[#ff7843]/40 bg-[#ff7843]/15 px-2 py-0.5 font-mono text-[9px] font-bold text-[#ff9769]">
                    {attachedFiles.length} selected
                  </span>
                )}
              </label>
              <span className="font-mono text-[10px] text-neutral-500">Select from PC</span>
            </div>

            {/* List of Attached Files */}
            {attachedFiles.length > 0 ? (
              <div className="space-y-2 rounded border border-[#26313d] bg-[#0a0d10] p-3">
                <div className="max-h-48 space-y-2 overflow-y-auto pr-1">
                  {attachedFiles.map((file) => (
                    <div
                      key={file.id}
                      className="flex items-center justify-between gap-3 rounded border border-[#222933] bg-[#10141a] p-2.5 transition hover:border-[#384654]"
                    >
                      <div className="flex items-center gap-2.5 min-w-0">
                        {file.isImage && file.dataUrl ? (
                          <img
                            src={file.dataUrl}
                            alt={file.name}
                            className="h-10 w-10 shrink-0 rounded object-cover border border-[#26313d]"
                          />
                        ) : (
                          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded bg-[#161c23] border border-[#26313d] text-neutral-400">
                            <FileText size={18} />
                          </div>
                        )}
                        <div className="min-w-0">
                          <p className="text-xs font-mono font-medium text-white truncate max-w-[240px]">
                            {file.name}
                          </p>
                          <div className="flex items-center gap-2 text-[10px] font-mono text-neutral-400 mt-0.5">
                            <span>{file.formattedSize}</span>
                            <span>•</span>
                            <span className="text-[#8c9ba8]">
                              {file.isImage ? "Photo / Screenshot" : "Document"}
                            </span>
                          </div>
                        </div>
                      </div>

                      <button
                        type="button"
                        onClick={() => handleRemoveFile(file.id)}
                        disabled={submitting}
                        className="rounded border border-[#26313d] bg-[#161c23] p-1.5 text-neutral-400 hover:border-red-500/40 hover:text-red-400 transition"
                        title="Remove file"
                      >
                        <Trash2 size={13} />
                      </button>
                    </div>
                  ))}
                </div>

                {/* Multi-file Controls */}
                <div className="pt-2 flex items-center justify-between border-t border-[#26313d] text-xs">
                  <button
                    type="button"
                    onClick={() => fileInputRef.current?.click()}
                    disabled={submitting}
                    className="inline-flex items-center gap-1.5 rounded border border-[#2b3542] bg-[#161c23] px-2.5 py-1 font-mono text-[10px] font-bold uppercase tracking-wider text-neutral-200 hover:text-white hover:border-[#384654] transition"
                  >
                    <Plus size={13} />
                    <span>Add More Files</span>
                  </button>
                  <button
                    type="button"
                    onClick={handleClearAllFiles}
                    disabled={submitting}
                    className="font-mono text-[10px] uppercase tracking-wider text-neutral-400 hover:text-red-400 transition"
                  >
                    Clear All
                  </button>
                </div>
              </div>
            ) : (
              /* Dropzone for Initial Selection */
              <div
                onClick={() => fileInputRef.current?.click()}
                className="group flex cursor-pointer flex-col items-center justify-center rounded border border-dashed border-[#2b3644] bg-[#0a0d10] p-6 text-center transition-all hover:border-[#ff7843]/60 hover:bg-[#12161c]"
              >
                <div className="flex h-10 w-10 items-center justify-center rounded border border-[#26313d] bg-[#161c23] text-neutral-400 group-hover:border-[#ff7843]/40 group-hover:text-[#ff7843] transition">
                  <UploadCloud size={20} />
                </div>
                <p className="mt-2 text-xs font-mono font-semibold text-white">
                  Click to select photo(s) or document(s) from your PC
                </p>
                <p className="mt-0.5 text-[10px] font-mono text-neutral-500">
                  Select multiple files: Photos, PDF, Office Docs, TXT, ZIP, Media (Max 25MB/file)
                </p>
              </div>
            )}

            <input
              ref={fileInputRef}
              type="file"
              multiple
              accept="image/*,application/pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt,.md,.csv,.json,.rtf,.odt,.zip,.mp4,.webm,.mp3,.wav"
              onChange={handleFilesChange}
              className="hidden"
            />
          </div>

          {/* Written Explanation / Notes */}
          <div className="space-y-1.5">
            <label
              htmlFor="proof-notes"
              className="block font-mono text-[10px] uppercase font-bold tracking-wider text-neutral-400 flex items-center justify-between"
            >
              <span>Explanation & Notes</span>
              <span className="text-[10px] text-neutral-500 lowercase">optional</span>
            </label>
            <textarea
              id="proof-notes"
              rows={3}
              value={textExplanation}
              onChange={(e) => setTextExplanation(e.target.value)}
              placeholder="Describe what you accomplished (e.g. 'Finished task requirements, attached screenshots and summary document above')..."
              className="w-full rounded border border-[#26313d] bg-[#0a0d10] p-3 text-xs text-white placeholder-neutral-600 transition focus:border-[#ff7843] focus:outline-none resize-none"
            />
          </div>

          {/* External Link (Optional) */}
          <div className="space-y-1.5">
            <label
              htmlFor="proof-link"
              className="block font-mono text-[10px] uppercase font-bold tracking-wider text-neutral-400 flex items-center justify-between"
            >
              <span className="flex items-center gap-1.5">
                <LinkIcon size={12} className="text-neutral-500" />
                Add External Link
              </span>
              <span className="text-[10px] text-neutral-500 lowercase">optional</span>
            </label>
            <input
              id="proof-link"
              type="url"
              value={externalLink}
              onChange={(e) => setExternalLink(e.target.value)}
              placeholder="e.g. https://github.com/my-pr or Google Drive link"
              className="w-full rounded border border-[#26313d] bg-[#0a0d10] px-3 py-2 text-xs text-white placeholder-neutral-600 transition focus:border-[#ff7843] focus:outline-none"
            />
          </div>

          {/* Destination Notice */}
          <div className="rounded border border-[#26313d] bg-[#0c0f14] p-3 text-xs text-neutral-400">
            {hasAssignedApprover ? (
              <div className="flex items-center gap-2">
                <Users size={14} className="text-[#ff7843] shrink-0" />
                <span>
                  Delivering to your assigned partner:{" "}
                  <strong className="text-white font-mono">{approverDisplay}</strong>
                </span>
              </div>
            ) : (
              <div className="flex items-center gap-2">
                <Users size={14} className="text-neutral-500 shrink-0" />
                <span>Your proof will be saved and visible to accountability partners.</span>
              </div>
            )}
          </div>

          {/* Action Buttons */}
          <div className="pt-2 flex items-center justify-end gap-2">
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="rounded border border-[#26313d] bg-[#161c23] px-4 py-2 font-mono text-xs font-bold uppercase tracking-wider text-neutral-300 hover:border-[#384654] hover:text-white transition"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={
                submitting ||
                (attachedFiles.length === 0 && !textExplanation.trim() && !externalLink.trim())
              }
              className="inline-flex items-center justify-center gap-2 rounded border border-[#ff7843] bg-[#ff7843] px-5 py-2 font-mono text-xs font-bold uppercase tracking-wider text-black shadow-lg hover:bg-[#ff8f5e] transition disabled:opacity-40 active:scale-95"
            >
              {submitting ? (
                <>
                  <Loader2 size={13} className="animate-spin text-black" />
                  <span>Submitting Proof...</span>
                </>
              ) : (
                <>
                  <Send size={13} />
                  <span>
                    Submit Proof {attachedFiles.length > 1 ? `(${attachedFiles.length} files)` : ""}
                  </span>
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>,
    document.body
  );
}
