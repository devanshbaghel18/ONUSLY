import { useState, useEffect } from "react";
import { Link, useNavigate } from "react-router-dom";
import AppLayout from "../components/AppLayout";
import {
  UploadCloud,
  FileText,
  Trash2,
  CheckCircle2,
  AlertCircle,
  Loader2,
  Users,
  UserCheck,
  Link2,
} from "lucide-react";
import { getGoals, submitProof, updateGoal } from "../lib/api";
import { getStoredCommunities } from "../lib/communities";
import { getStoredFriends } from "../lib/friendsChat";

export default function SubmitProof() {
  const navigate = useNavigate();
  const [goals, setGoals] = useState([]);
  const [selectedGoalId, setSelectedGoalId] = useState("");
  const [loading, setLoading] = useState(true);

  // Upload & Proof Data
  const [uploadedDocs, setUploadedDocs] = useState([]);
  const [verificationUrl, setVerificationUrl] = useState("");
  const [explanation, setExplanation] = useState("");

  // Share Option: 'community' | 'friend'
  const [shareTarget, setShareTarget] = useState("community");
  const [selectedCommunity, setSelectedCommunity] = useState("");
  const [selectedFriend, setSelectedFriend] = useState("");

  const [submitting, setSubmitting] = useState(false);
  const [notice, setNotice] = useState({ error: "", success: "" });

  const communities = getStoredCommunities();
  const friends = getStoredFriends();

  // Load Active Goals on Mount
  useEffect(() => {
    getGoals()
      .then((data) => {
        const active = data.filter((g) => g.status === "active" || g.status === "proof_submitted");
        setGoals(active);
        if (active.length > 0) {
          setSelectedGoalId(active[0].id);
          if (active[0].approvalType === "friend") {
            setShareTarget("friend");
            setSelectedFriend(active[0].approverEmail || "");
          } else if (active[0].approvalType === "community") {
            setShareTarget("community");
          }
        }
      })
      .catch((err) => {
        console.error("Failed to load goals:", err);
      })
      .finally(() => setLoading(false));
  }, []);

  // Handle File Input Selection
  const handleFileUpload = (e) => {
    const files = Array.from(e.target.files || []);
    if (!files.length) return;

    const newDocs = files.map((file) => ({
      id: `doc-${Date.now()}-${Math.random().toString(36).substr(2, 5)}`,
      name: file.name,
      size: `${(file.size / 1024).toFixed(1)} KB`,
      type: file.type || "document",
      uploadedAt: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    }));

    setUploadedDocs((prev) => [...prev, ...newDocs]);
  };

  const handleRemoveDoc = (docId) => {
    setUploadedDocs((prev) => prev.filter((d) => d.id !== docId));
  };

  const handleSubmitProof = async (e) => {
    e.preventDefault();
    setNotice({ error: "", success: "" });

    if (!selectedGoalId) {
      setNotice({ error: "Please select an active goal to submit proof for.", success: "" });
      return;
    }

    if (uploadedDocs.length === 0 && !verificationUrl.trim() && !explanation.trim()) {
      setNotice({
        error: "Please upload at least one document, provide a verification link, or write an explanation.",
        success: "",
      });
      return;
    }

    setSubmitting(true);
    try {
      // 1. Submit proof on backend
      const summaryText = [
        explanation.trim(),
        uploadedDocs.length > 0 ? `[Documents Attached: ${uploadedDocs.map((d) => d.name).join(", ")}]` : "",
      ]
        .filter(Boolean)
        .join(" | ");

      await submitProof(selectedGoalId, {
        proofType: uploadedDocs.length > 0 ? "document" : "text",
        textExplanation: summaryText || "Proof documents uploaded.",
        externalLink: verificationUrl.trim(),
        photoUrl: uploadedDocs[0]?.name || "",
      });

      // 2. Ensure goal's accountability setting reflects sharing choice
      if (shareTarget === "friend" && selectedFriend) {
        await updateGoal(selectedGoalId, {
          approvalType: "friend",
          approverEmail: selectedFriend,
        });
      } else if (shareTarget === "community") {
        await updateGoal(selectedGoalId, {
          approvalType: "community",
        });
      }

      setNotice({
        error: "",
        success: "Verification proof submitted successfully! Pending review to unlock.",
      });

      setUploadedDocs([]);
      setVerificationUrl("");
      setExplanation("");

      setTimeout(() => {
        navigate("/dashboard");
      }, 2500);
    } catch (err) {
      console.error("Failed to submit proof:", err);
      setNotice({ error: err.message || "Failed to submit verification proof", success: "" });
    } finally {
      setSubmitting(false);
    }
  };

  const selectedGoal = goals.find((g) => g.id === selectedGoalId);

  return (
    <AppLayout>
      <div className="mx-auto max-w-4xl space-y-8">
        {/* Header */}
        <div className="border-b border-[#2A2A2A] pb-5">
          <h1 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
            Submit Verification Proof
          </h1>
          <p className="mt-1 text-sm text-[#A3A3A3]">
            Upload documents, code, or links to prove completion and unlock your blocked apps.
          </p>
        </div>

        {/* Notices */}
        {notice.error && (
          <div className="flex items-center gap-2 rounded-2xl border border-[#444444] bg-[#1E1E1E] p-4 text-xs font-semibold text-white">
            <AlertCircle size={16} />
            <span>{notice.error}</span>
          </div>
        )}
        {notice.success && (
          <div className="flex items-center gap-2 rounded-2xl border border-white bg-white/10 p-4 text-xs font-semibold text-white">
            <CheckCircle2 size={16} />
            <span>{notice.success}</span>
          </div>
        )}

        <form onSubmit={handleSubmitProof} className="space-y-6">
          {/* 1. SELECT GOAL */}
          <div className="rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-6 space-y-3">
            <label className="block text-xs font-bold uppercase tracking-wider text-[#A3A3A3]">
              1. Select Active Lockout Goal
            </label>

            {loading ? (
              <div className="h-10 animate-pulse rounded-xl bg-[#222222]" />
            ) : goals.length === 0 ? (
              <div className="py-4 text-center">
                <p className="text-xs text-[#737373]">No active lockout goals found.</p>
                <Link
                  to="/goals"
                  className="mt-2 inline-block text-xs font-semibold text-white underline"
                >
                  Create an accountability goal first →
                </Link>
              </div>
            ) : (
              <select
                value={selectedGoalId}
                onChange={(e) => setSelectedGoalId(e.target.value)}
                className="w-full rounded-xl border border-[#333333] bg-[#121212] px-4 py-3 text-sm text-white focus:border-white focus:outline-none"
              >
                {goals.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.title} ({g.status === "proof_submitted" ? "Pending Review" : "Active Lock"})
                  </option>
                ))}
              </select>
            )}

            {selectedGoal && selectedGoal.description && (
              <p className="text-xs text-[#737373]">
                Goal details: {selectedGoal.description}
              </p>
            )}
          </div>

          {/* 2. UPLOAD DOCUMENT AREA */}
          <div className="rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-6 space-y-4">
            <label className="block text-xs font-bold uppercase tracking-wider text-[#A3A3A3]">
              2. Upload Verification Document
            </label>

            {/* Drag & Drop File Upload Box */}
            <label className="flex cursor-pointer flex-col items-center justify-center rounded-2xl border-2 border-dashed border-[#3A3A3A] bg-[#141414] py-10 transition-colors hover:border-white hover:bg-[#1A1A1A]">
              <div className="flex h-12 w-12 items-center justify-center rounded-2xl border border-[#333333] bg-[#222222] text-white mb-3">
                <UploadCloud size={24} />
              </div>
              <p className="text-sm font-semibold text-white">
                Click to upload document or screenshot
              </p>
              <p className="mt-1 text-xs text-[#737373]">
                Supports PDF, PNG, JPG, Markdown, TXT
              </p>
              <input
                type="file"
                multiple
                onChange={handleFileUpload}
                className="hidden"
                accept="image/*,.pdf,.doc,.docx,.txt"
              />
            </label>

            {/* Optional Verification URL */}
            <div>
              <label className="block text-xs font-semibold text-[#A3A3A3] mb-1.5">
                Or Verification Link (GitHub PR, Google Doc, etc.)
              </label>
              <div className="relative">
                <Link2
                  size={15}
                  className="absolute left-3.5 top-1/2 -translate-y-1/2 text-[#737373]"
                />
                <input
                  type="url"
                  placeholder="https://github.com/... or https://docs.google.com/..."
                  value={verificationUrl}
                  onChange={(e) => setVerificationUrl(e.target.value)}
                  className="w-full rounded-xl border border-[#333333] bg-[#121212] py-2.5 pl-10 pr-4 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
                />
              </div>
            </div>

            {/* Notes / Explanation */}
            <div>
              <label className="block text-xs font-semibold text-[#A3A3A3] mb-1.5">
                Work Summary / Explanation
              </label>
              <textarea
                rows={3}
                placeholder="Describe the proof and work done to verify this goal..."
                value={explanation}
                onChange={(e) => setExplanation(e.target.value)}
                className="w-full rounded-xl border border-[#333333] bg-[#121212] px-3.5 py-2.5 text-xs text-white placeholder-[#737373] focus:border-white focus:outline-none"
              />
            </div>

            {/* =======================================================
                DOWNSIDE: UPLOADED DOCUMENTS VIEWER
            ======================================================= */}
            <div className="border-t border-[#2A2A2A] pt-4">
              <span className="block text-xs font-bold uppercase tracking-wider text-[#A3A3A3] mb-3">
                Uploaded Documents ({uploadedDocs.length})
              </span>

              {uploadedDocs.length === 0 ? (
                <p className="text-xs text-[#737373] italic">
                  No documents attached yet. Choose files above to see them listed here.
                </p>
              ) : (
                <div className="space-y-2">
                  {uploadedDocs.map((doc) => (
                    <div
                      key={doc.id}
                      className="flex items-center justify-between rounded-xl border border-[#333333] bg-[#121212] p-3 text-xs"
                    >
                      <div className="flex items-center gap-3 min-w-0">
                        <div className="flex h-8 w-8 items-center justify-center rounded-lg border border-[#444444] bg-[#222222] text-white">
                          <FileText size={16} />
                        </div>
                        <div className="min-w-0 flex-1">
                          <p className="truncate font-semibold text-white">
                            {doc.name}
                          </p>
                          <p className="text-[10px] text-[#737373]">
                            {doc.size} • Uploaded at {doc.uploadedAt}
                          </p>
                        </div>
                      </div>

                      <button
                        type="button"
                        onClick={() => handleRemoveDoc(doc.id)}
                        className="rounded-lg p-1.5 text-[#737373] hover:text-white"
                        title="Remove file"
                      >
                        <Trash2 size={15} />
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* 3. SHARE WITH COMMUNITY OR FRIEND */}
          <div className="rounded-2xl border border-[#2B2B2B] bg-[#1A1A1A] p-6 space-y-4">
            <label className="block text-xs font-bold uppercase tracking-wider text-[#A3A3A3]">
              3. Share Proof With
            </label>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              {/* Option A: Share with Community */}
              <div
                onClick={() => setShareTarget("community")}
                className={`cursor-pointer rounded-xl border p-4 transition-all ${
                  shareTarget === "community"
                    ? "border-white bg-[#242424]"
                    : "border-[#333333] bg-[#141414] hover:border-[#555555]"
                }`}
              >
                <div className="flex items-center gap-2">
                  <Users size={16} className="text-white" />
                  <span className="text-xs font-bold text-white">
                    Share with Community Quorum
                  </span>
                </div>
                <p className="mt-1 text-[11px] text-[#A3A3A3]">
                  Fellow members in your community review and verify your upload.
                </p>

                {shareTarget === "community" && communities.length > 0 && (
                  <div className="mt-3 pt-2 border-t border-[#333333]">
                    <select
                      value={selectedCommunity}
                      onChange={(e) => setSelectedCommunity(e.target.value)}
                      className="w-full rounded-lg border border-[#444444] bg-[#121212] px-3 py-1.5 text-xs text-white focus:outline-none"
                    >
                      <option value="">Select community...</option>
                      {communities.map((c) => (
                        <option key={c.id} value={c.name}>
                          {c.name}
                        </option>
                      ))}
                    </select>
                  </div>
                )}
              </div>

              {/* Option B: Share with Friend */}
              <div
                onClick={() => setShareTarget("friend")}
                className={`cursor-pointer rounded-xl border p-4 transition-all ${
                  shareTarget === "friend"
                    ? "border-white bg-[#242424]"
                    : "border-[#333333] bg-[#141414] hover:border-[#555555]"
                }`}
              >
                <div className="flex items-center gap-2">
                  <UserCheck size={16} className="text-white" />
                  <span className="text-xs font-bold text-white">
                    Share with Friend
                  </span>
                </div>
                <p className="mt-1 text-[11px] text-[#A3A3A3]">
                  A dedicated 1-on-1 partner inspects your proof to unlock the lockout.
                </p>

                {shareTarget === "friend" && (
                  <div className="mt-3 pt-2 border-t border-[#333333] space-y-2">
                    {friends.length > 0 ? (
                      <select
                        value={selectedFriend}
                        onChange={(e) => setSelectedFriend(e.target.value)}
                        className="w-full rounded-lg border border-[#444444] bg-[#121212] px-3 py-1.5 text-xs text-white focus:outline-none"
                      >
                        <option value="">Choose from your friends...</option>
                        {friends.map((f) => (
                          <option key={f.id} value={f.email || f.name}>
                            {f.name} {f.email ? `(${f.email})` : ""}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <input
                        type="email"
                        placeholder="Friend's email address"
                        value={selectedFriend}
                        onChange={(e) => setSelectedFriend(e.target.value)}
                        className="w-full rounded-lg border border-[#444444] bg-[#121212] px-3 py-1.5 text-xs text-white placeholder-[#737373] focus:outline-none"
                      />
                    )}
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* 4. SUBMIT ACTION */}
          <div className="flex items-center justify-end gap-3 pt-4">
            <Link
              to="/dashboard"
              className="rounded-xl border border-[#333333] bg-transparent px-5 py-2.5 text-xs font-semibold text-[#A3A3A3] hover:text-white"
            >
              Cancel
            </Link>
            <button
              type="submit"
              disabled={submitting || goals.length === 0}
              className="flex items-center gap-2 rounded-xl bg-white px-7 py-3 text-xs font-bold text-black transition-all hover:bg-neutral-200 disabled:opacity-40"
            >
              {submitting ? (
                <>
                  <Loader2 size={16} className="animate-spin" />
                  Submitting Proof...
                </>
              ) : (
                <>
                  <UploadCloud size={16} />
                  Submit Proof for Review
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </AppLayout>
  );
}
