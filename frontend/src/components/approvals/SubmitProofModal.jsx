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
  MessageSquare,
  Send,
  Eye,
} from "lucide-react";
import { getUser } from "../../lib/auth";
import { submitProof, updateGoal } from "../../lib/api";
import { getStoredFriends, sendChatMessage } from "../../lib/friendsChat";
import { getStoredCommunities, sendCommunityMessage } from "../../lib/communities";

function generateMsgId(prefix = "msg") {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).substring(2, 7)}`;
}

// Compress image via HTML canvas to ensure payload stays under 2MB safely
function processImageFile(file, maxDimension = 1600, quality = 0.85) {
  return new Promise((resolve, reject) => {
    if (!file.type.startsWith("image/")) {
      // Non-image file (e.g. video or PDF) -> Read raw Data URL
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
  const [fileDataUrl, setFileDataUrl] = useState("");
  const [fileName, setFileName] = useState("");
  const [fileSize, setFileSize] = useState("");
  const [textExplanation, setTextExplanation] = useState("");
  const [externalLink, setExternalLink] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  // Target destination state (if goal doesn't already have an assigned approver)
  const [shareTarget, setShareTarget] = useState("friend");
  const [selectedFriendId, setSelectedFriendId] = useState("");
  const [selectedCommunityId, setSelectedCommunityId] = useState("");

  const friends = getStoredFriends();
  const communities = getStoredCommunities();
  const fileInputRef = useRef(null);

  useEffect(() => {
    if (isOpen) {
      setFileDataUrl("");
      setFileName("");
      setFileSize("");
      setTextExplanation("");
      setExternalLink("");
      setError("");
      setSubmitting(false);

      if (friends.length > 0) setSelectedFriendId(friends[0].id);
      if (communities.length > 0) setSelectedCommunityId(communities[0].id);
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

  const handleFileChange = async (e) => {
    const file = e.target.files?.[0];
    if (!file) return;

    try {
      setError("");
      const sizeFormatted =
        file.size > 1024 * 1024
          ? `${(file.size / (1024 * 1024)).toFixed(1)} MB`
          : `${Math.round(file.size / 1024)} KB`;

      setFileName(file.name);
      setFileSize(sizeFormatted);

      const processedDataUrl = await processImageFile(file);
      setFileDataUrl(processedDataUrl);
    } catch (err) {
      console.error("Failed to read file:", err);
      setError("Failed to read file. Please select a valid image or document.");
    }
  };

  const handleRemoveFile = () => {
    setFileDataUrl("");
    setFileName("");
    setFileSize("");
    if (fileInputRef.current) fileInputRef.current.value = "";
  };

  const handleSubmit = async (e) => {
    e?.preventDefault();
    if (submitting) return;

    // Validation: At least one proof element must be provided
    const trimmedNotes = textExplanation.trim();
    const trimmedLink = externalLink.trim();

    if (!fileDataUrl && !trimmedNotes && !trimmedLink) {
      setError("Please attach a photo, enter an explanation, or provide a link to submit proof.");
      return;
    }

    setSubmitting(true);
    setError("");

    try {
      const currentUser = getUser();
      const goalId = goal.id;

      // Determine proof type
      let proofType = "text";
      if (fileDataUrl) proofType = "photo";
      else if (trimmedLink) proofType = "link";

      // Submit proof to backend REST API
      const result = await submitProof(goalId, {
        proofType,
        photoUrl: fileDataUrl,
        textExplanation: trimmedNotes,
        externalLink: trimmedLink,
      });

      // Send chat notification to partner in background
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
              text: `Submitted verification proof for goal: "${goal.title}"`,
              isProof: true,
              goalId,
              goalTitle: goal.title,
              ownerId: currentUser?.id || "",
              proofType,
              photoUrl: fileDataUrl,
              textExplanation: trimmedNotes,
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
          {/* File Upload / Image Picker from PC */}
          <div>
            <label className="block text-xs font-semibold text-white mb-2 flex items-center justify-between">
              <span>Photo, Screenshot or File</span>
              <span className="text-[11px] font-normal text-neutral-400">From PC</span>
            </label>

            {fileDataUrl ? (
              <div className="relative overflow-hidden rounded-xl border border-white/10 bg-black/40 p-3">
                <div className="flex items-center gap-3">
                  {fileDataUrl.startsWith("data:image/") ? (
                    <img
                      src={fileDataUrl}
                      alt="Selected Proof"
                      className="h-16 w-16 rounded-lg object-cover border border-white/10"
                    />
                  ) : (
                    <div className="flex h-16 w-16 items-center justify-center rounded-lg bg-white/5 border border-white/10 text-neutral-400">
                      <FileText size={24} />
                    </div>
                  )}

                  <div className="flex-1 min-w-0">
                    <p className="text-xs font-semibold text-white truncate">{fileName}</p>
                    <p className="text-[11px] text-neutral-400 mt-0.5">{fileSize}</p>
                    <span className="inline-flex items-center gap-1 text-[10px] text-emerald-400 mt-1">
                      <CheckCircle2 size={11} /> Ready to submit
                    </span>
                  </div>

                  <button
                    type="button"
                    onClick={handleRemoveFile}
                    disabled={submitting}
                    className="rounded-lg p-2 text-neutral-400 hover:bg-white/10 hover:text-rose-400 transition"
                    title="Remove File"
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
              </div>
            ) : (
              <div
                onClick={() => fileInputRef.current?.click()}
                className="group flex cursor-pointer flex-col items-center justify-center rounded-xl border border-dashed border-white/20 bg-white/[0.02] p-6 text-center transition-all hover:border-white/40 hover:bg-white/[0.04]"
              >
                <div className="flex h-10 w-10 items-center justify-center rounded-full bg-white/5 text-neutral-400 group-hover:scale-105 group-hover:text-white transition">
                  <UploadCloud size={20} />
                </div>
                <p className="mt-2 text-xs font-semibold text-white">
                  Click to select photo or screenshot from your PC
                </p>
                <p className="mt-0.5 text-[11px] text-neutral-500">
                  PNG, JPG, WebP, GIF, or PDF (Max 2MB)
                </p>
              </div>
            )}

            <input
              ref={fileInputRef}
              type="file"
              accept="image/*,video/*,application/pdf"
              onChange={handleFileChange}
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
              placeholder="Describe what you accomplished (e.g. 'Finished chapters 1-3, attached summary notes above')..."
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
              disabled={submitting || (!fileDataUrl && !textExplanation.trim() && !externalLink.trim())}
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
                  <span>Submit Proof for Verification</span>
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
