import { useState, useEffect, useRef } from "react";
import {
  X,
  UploadCloud,
  Camera,
  FileText,
  Link as LinkIcon,
  CheckCircle2,
  AlertCircle,
  Loader2,
  Trash2,
  Users,
  Plus,
  Send,
  Eye,
  Layers,
} from "lucide-react";
import { getUser } from "../../lib/auth";
import { submitProof } from "../../lib/api";
import { getStoredFriends } from "../../lib/friendsChat";
import { getStoredCommunities } from "../../lib/communities";

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

// Compress single image via HTML canvas to ensure reasonable size
function processImageFile(file, maxDimension = 1400, quality = 0.82) {
  return new Promise((resolve, reject) => {
    if (!file.type.startsWith("image/")) {
      // Non-image file (e.g. PDF or document) -> Read raw Data URL
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result);
      reader.onerror = reject;
      reader.readAsDataURL(file);
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
    reader.onerror = reject;
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

  const friends = getStoredFriends();
  const communities = getStoredCommunities();
  const fileInputRef = useRef(null);

  useEffect(() => {
    if (isOpen) {
      setAttachedFiles([]);
      setTextExplanation("");
      setExternalLink("");
      setError("");
      setSubmitting(false);
    }
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

      // Prepare final photoUrl / composite evidence
      let finalPhotoUrl = "";
      const imageFiles = attachedFiles.filter((f) => f.isImage && f.dataUrl);
      const docFiles = attachedFiles.filter((f) => !f.isImage && f.dataUrl);

      if (imageFiles.length > 0) {
        if (imageFiles.length === 1) {
          finalPhotoUrl = imageFiles[0].dataUrl;
        } else {
          // Combine multiple images into single vertical inspection canvas
          finalPhotoUrl = await compositeImages(imageFiles.map((f) => f.dataUrl));
        }
      } else if (docFiles.length > 0) {
        finalPhotoUrl = docFiles[0].dataUrl;
      }

      // Determine proofType
      let proofType = "text";
      if (imageFiles.length > 0) proofType = "photo";
      else if (docFiles.length > 0) proofType = "document";
      else if (trimmedLink) proofType = "link";

      // Append attached documents manifest to notes if multiple files exist
      let finalExplanation = trimmedNotes;
      if (attachedFiles.length > 0) {
        const fileManifest = attachedFiles
          .map((f, i) => `  ${i + 1}. ${f.name} (${f.formattedSize})`)
          .join("\n");
        const suffix = `\n\n[Attached Proof Documents (${attachedFiles.length})]:\n${fileManifest}`;
        finalExplanation = trimmedNotes ? `${trimmedNotes}${suffix}` : `[Attached Proof Documents (${attachedFiles.length})]:\n${fileManifest}`;
      }

      // Submit proof to backend REST API
      const result = await submitProof(goalId, {
        proofType,
        photoUrl: finalPhotoUrl,
        textExplanation: finalExplanation,
        externalLink: trimmedLink,
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
              text: `Submitted verification proof for goal: "${goal.title}" (${attachedFiles.length} file(s) attached)`,
              isProof: true,
              goalId,
              goalTitle: goal.title,
              ownerId: currentUser?.id || "",
              proofType,
              photoUrl: finalPhotoUrl,
              textExplanation: finalExplanation,
              fileCount: attachedFiles.length,
            },
          });
        }
      } catch (wsErr) {
        console.warn("Background chat notification skipped:", wsErr.message);
      }

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

  return (
    <div
      role="dialog"
      aria-modal="true"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/85 p-4 backdrop-blur-md animate-in fade-in duration-150"
      onClick={(e) => {
        if (e.target === e.currentTarget && !submitting) onClose();
      }}
    >
      <div className="relative w-full max-w-lg rounded-2xl border border-white/10 bg-[#141418] p-6 shadow-2xl animate-in zoom-in-95 duration-150 max-h-[90vh] overflow-y-auto">
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
        <div className="flex items-center gap-3 border-b border-white/10 pb-4">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl border border-indigo-500/30 bg-indigo-500/10 text-indigo-400">
            <Camera size={22} />
          </div>
          <div className="min-w-0 pr-6">
            <h2 className="text-lg font-bold text-white tracking-tight">
              Submit Proof for Verification
            </h2>
            <p className="truncate text-xs text-neutral-400">
              Attach evidence of your completed work for "{goal.title}"
            </p>
          </div>
        </div>

        {/* Error Alert */}
        {error && (
          <div className="mt-4 flex items-center gap-2 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-300">
            <AlertCircle size={15} className="shrink-0 text-red-400" />
            <span>{error}</span>
          </div>
        )}

        <form onSubmit={handleSubmit} className="mt-5 space-y-4">
          {/* Multi-File Upload / Documents Picker from PC */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="text-xs font-semibold text-white flex items-center gap-1.5">
                <span>Proof Documents & Photos</span>
                {attachedFiles.length > 0 && (
                  <span className="rounded-full bg-indigo-500/20 px-2 py-0.5 text-[10px] font-bold text-indigo-300">
                    {attachedFiles.length} selected
                  </span>
                )}
              </label>
              <span className="text-[11px] font-normal text-neutral-400">Select from PC</span>
            </div>

            {/* List of Attached Files */}
            {attachedFiles.length > 0 ? (
              <div className="space-y-2 rounded-xl border border-white/10 bg-black/40 p-3">
                <div className="max-h-48 space-y-2 overflow-y-auto pr-1">
                  {attachedFiles.map((file, idx) => (
                    <div
                      key={file.id}
                      className="flex items-center justify-between gap-3 rounded-lg border border-white/10 bg-white/[0.03] p-2.5 transition hover:border-white/20"
                    >
                      <div className="flex items-center gap-2.5 min-w-0">
                        {file.isImage && file.dataUrl ? (
                          <img
                            src={file.dataUrl}
                            alt={file.name}
                            className="h-10 w-10 shrink-0 rounded-md object-cover border border-white/10"
                          />
                        ) : (
                          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-white/5 border border-white/10 text-neutral-300">
                            <FileText size={18} />
                          </div>
                        )}
                        <div className="min-w-0">
                          <p className="text-xs font-medium text-white truncate max-w-[240px]">
                            {file.name}
                          </p>
                          <div className="flex items-center gap-2 text-[10px] text-neutral-400 mt-0.5">
                            <span>{file.formattedSize}</span>
                            <span>•</span>
                            <span className="text-indigo-300">
                              {file.isImage ? "Photo / Screenshot" : "Document"}
                            </span>
                          </div>
                        </div>
                      </div>

                      <button
                        type="button"
                        onClick={() => handleRemoveFile(file.id)}
                        disabled={submitting}
                        className="rounded-lg p-1.5 text-neutral-400 hover:bg-rose-500/10 hover:text-rose-400 transition"
                        title="Remove file"
                      >
                        <Trash2 size={14} />
                      </button>
                    </div>
                  ))}
                </div>

                {/* Multi-file Controls */}
                <div className="pt-2 flex items-center justify-between border-t border-white/10 text-xs">
                  <button
                    type="button"
                    onClick={() => fileInputRef.current?.click()}
                    disabled={submitting}
                    className="inline-flex items-center gap-1.5 rounded-lg border border-white/15 bg-white/5 px-2.5 py-1.5 text-[11px] font-semibold text-neutral-200 hover:bg-white/10 hover:text-white transition"
                  >
                    <Plus size={13} />
                    <span>Add More Files</span>
                  </button>
                  <button
                    type="button"
                    onClick={handleClearAllFiles}
                    disabled={submitting}
                    className="text-[11px] text-neutral-400 hover:text-rose-400 transition"
                  >
                    Clear All
                  </button>
                </div>
              </div>
            ) : (
              /* Dropzone for Initial Selection */
              <div
                onClick={() => fileInputRef.current?.click()}
                className="group flex cursor-pointer flex-col items-center justify-center rounded-xl border border-dashed border-white/20 bg-white/[0.02] p-6 text-center transition-all hover:border-white/40 hover:bg-white/[0.04]"
              >
                <div className="flex h-11 w-11 items-center justify-center rounded-full bg-white/5 text-neutral-400 group-hover:scale-105 group-hover:text-white transition">
                  <UploadCloud size={22} />
                </div>
                <p className="mt-2 text-xs font-semibold text-white">
                  Click to select photo(s) or document(s) from your PC
                </p>
                <p className="mt-0.5 text-[11px] text-neutral-500">
                  Select multiple files: PNG, JPG, WebP, PDF or Docs (Max 2MB per upload)
                </p>
              </div>
            )}

            <input
              ref={fileInputRef}
              type="file"
              multiple
              accept="image/*,application/pdf,.doc,.docx,.txt"
              onChange={handleFilesChange}
              className="hidden"
            />
          </div>

          {/* Written Explanation / Notes */}
          <div className="space-y-1.5">
            <label
              htmlFor="proof-notes"
              className="block text-xs font-semibold text-white flex items-center justify-between"
            >
              <span>Explanation & Notes</span>
              <span className="text-[11px] font-normal text-neutral-400">Optional</span>
            </label>
            <textarea
              id="proof-notes"
              rows={3}
              value={textExplanation}
              onChange={(e) => setTextExplanation(e.target.value)}
              placeholder="Describe what you accomplished (e.g. 'Finished task requirements, attached screenshots and summary document above')..."
              className="w-full rounded-xl border border-white/10 bg-white/[0.03] p-3 text-xs text-white placeholder-neutral-500 transition focus:border-white/30 focus:outline-none focus:ring-1 focus:ring-white/20 resize-none"
            />
          </div>

          {/* External Link (Optional) */}
          <div className="space-y-1.5">
            <label
              htmlFor="proof-link"
              className="block text-xs font-semibold text-white flex items-center justify-between"
            >
              <span className="flex items-center gap-1.5">
                <LinkIcon size={12} className="text-neutral-400" />
                Add External Link
              </span>
              <span className="text-[11px] font-normal text-neutral-400">Optional</span>
            </label>
            <input
              id="proof-link"
              type="url"
              value={externalLink}
              onChange={(e) => setExternalLink(e.target.value)}
              placeholder="e.g. https://github.com/my-pr or Google Drive link"
              className="w-full rounded-xl border border-white/10 bg-white/[0.03] px-3 py-2.5 text-xs text-white placeholder-neutral-500 transition focus:border-white/30 focus:outline-none focus:ring-1 focus:ring-white/20"
            />
          </div>

          {/* Destination Notice */}
          <div className="rounded-xl border border-white/5 bg-white/[0.02] p-3 text-xs text-neutral-400">
            {hasAssignedApprover ? (
              <div className="flex items-center gap-2">
                <Users size={14} className="text-indigo-400 shrink-0" />
                <span>
                  Delivering to your assigned partner:{" "}
                  <strong className="text-white">{approverDisplay}</strong>
                </span>
              </div>
            ) : (
              <div className="flex items-center gap-2">
                <Users size={14} className="text-neutral-400 shrink-0" />
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
              className="rounded-xl border border-white/10 bg-white/5 px-4 py-2.5 text-xs font-semibold text-neutral-300 hover:bg-white/10 hover:text-white transition"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={
                submitting ||
                (attachedFiles.length === 0 && !textExplanation.trim() && !externalLink.trim())
              }
              className="inline-flex items-center justify-center gap-2 rounded-xl bg-white px-5 py-2.5 text-xs font-bold text-black shadow-lg hover:bg-neutral-200 transition disabled:opacity-40"
            >
              {submitting ? (
                <>
                  <Loader2 size={14} className="animate-spin" />
                  <span>Submitting Proof...</span>
                </>
              ) : (
                <>
                  <Send size={14} />
                  <span>
                    Submit Proof {attachedFiles.length > 1 ? `(${attachedFiles.length} files)` : ""}
                  </span>
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
