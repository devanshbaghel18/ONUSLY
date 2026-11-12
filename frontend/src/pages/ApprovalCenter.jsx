import { useState, useEffect, useCallback, useMemo } from "react";
import AppLayout from "../components/AppLayout";
import {
  CheckSquare,
  Loader2,
  AlertCircle,
  RefreshCw,
  Search,
  CheckCircle2,
  XCircle,
  Shield,
  Target,
  Sparkles,
} from "lucide-react";
import { getApproverInbox, decideApproval } from "../lib/api";
import ApprovalCard from "../components/approvals/ApprovalCard";
import EvidenceLightbox from "../components/approvals/EvidenceLightbox";

const TABS = [
  { id: "pending", label: "Pending Review" },
  { id: "approved", label: "Approved" },
  { id: "rejected", label: "Rejected" },
  { id: "all", label: "All Goals" },
];

export default function ApprovalCenter() {
  const [items, setItems] = useState([]);
  const [totalCount, setTotalCount] = useState(0);
  const [pendingCount, setPendingCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  const [activeTab, setActiveTab] = useState("pending");
  const [searchQuery, setSearchQuery] = useState("");
  const [notice, setNotice] = useState("");
  const [lightboxData, setLightboxData] = useState(null);

  const fetchInbox = useCallback(async (tab = activeTab, isManualRefresh = false) => {
    if (isManualRefresh) setRefreshing(true);
    else setLoading(true);
    setError("");

    try {
      const data = await getApproverInbox(tab);
      const fetchedItems = data?.items || [];
      const fetchedTotal = typeof data?.total === "number" ? data.total : fetchedItems.length;

      setItems(fetchedItems);
      setTotalCount(fetchedTotal);

      // If viewing pending, update pendingCount directly
      if (tab === "pending") {
        setPendingCount(fetchedTotal);
      } else {
        // Also quietly fetch pending count for the tab badge if not on pending tab
        getApproverInbox("pending")
          .then((res) => {
            if (typeof res?.total === "number") {
              setPendingCount(res.total);
            }
          })
          .catch(() => {});
      }
    } catch (err) {
      setError(err?.message || "Failed to load approval inbox");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [activeTab]);

  useEffect(() => {
    let mounted = true;
    fetchInbox(activeTab);
    return () => {
      mounted = false;
    };
  }, [activeTab, fetchInbox]);

  const showNotice = (msg, duration = 4000) => {
    setNotice(msg);
    setTimeout(() => setNotice(""), duration);
  };

  // Filter items client-side by search query
  const filteredItems = useMemo(() => {
    if (!searchQuery.trim()) return items;
    const q = searchQuery.toLowerCase().trim();
    return items.filter((item) => {
      const titleMatch = item.title?.toLowerCase().includes(q);
      const descMatch = item.description?.toLowerCase().includes(q);
      const nameMatch = item.ownerName?.toLowerCase().includes(q);
      const handleMatch = item.ownerHandle?.toLowerCase().includes(q);
      const emailMatch = item.ownerEmail?.toLowerCase().includes(q);
      const appsMatch = item.blockedApps?.some((a) => a.toLowerCase().includes(q));
      const domainsMatch = item.blockedDomains?.some((d) => d.toLowerCase().includes(q));
      return (
        titleMatch ||
        descMatch ||
        nameMatch ||
        handleMatch ||
        emailMatch ||
        appsMatch ||
        domainsMatch
      );
    });
  }, [items, searchQuery]);

  // Handlers for cards (Sub-part 2.3 & 2.4 hooks)
  const handleInspectPhoto = (photo, item) => {
    setLightboxData({ item, proof: photo });
  };

  const handleApprove = async (item) => {
    if (!item?.goalId || !item?.latestProof?.id) {
      alert("Missing proof details to approve this goal.");
      return;
    }
    const confirmed = window.confirm(
      `Approve goal "${item.title}" for ${item.ownerName || item.ownerHandle || "your partner"}?\n\nThis will immediately unlock their blocked applications.`
    );
    if (!confirmed) return;

    try {
      await decideApproval(item.goalId, item.latestProof.id, {
        ownerId: item.ownerId,
        status: "approved",
        comment: "Approved verification proof.",
      });

      // Optimistic removal from pending tab
      setItems((prev) => prev.filter((i) => i.goalId !== item.goalId));
      setPendingCount((c) => Math.max(0, c - 1));
      showNotice(`✅ "${item.title}" approved! Targets unlocked.`);
    } catch (err) {
      alert("Approval failed: " + (err?.message || "Unknown error"));
    }
  };

  const handleReject = async (item) => {
    if (!item?.goalId || !item?.latestProof?.id) {
      alert("Missing proof details to reject this goal.");
      return;
    }

    const reason = window.prompt(
      `Reject proof for "${item.title}"?\n\nPlease enter feedback explaining why this proof does not meet requirements (minimum 5 characters):`
    );

    if (reason === null) return; // User cancelled
    if (reason.trim().length < 5) {
      alert("Rejection reason must be at least 5 characters long so your partner knows what to fix.");
      return;
    }

    try {
      await decideApproval(item.goalId, item.latestProof.id, {
        ownerId: item.ownerId,
        status: "rejected",
        comment: reason.trim(),
      });

      // Optimistic removal from pending tab
      setItems((prev) => prev.filter((i) => i.goalId !== item.goalId));
      setPendingCount((c) => Math.max(0, c - 1));
      showNotice(`⚠️ Proof for "${item.title}" rejected. Feedback saved.`);
    } catch (err) {
      alert("Rejection failed: " + (err?.message || "Unknown error"));
    }
  };

  return (
    <AppLayout>
      <div className="mx-auto max-w-5xl px-4 py-8 sm:px-6 lg:px-8">
        {/* Notice toast */}
        {notice && (
          <div className="mb-6 flex items-center justify-between rounded-xl border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm font-medium text-emerald-300 transition-all">
            <span>{notice}</span>
            <button
              onClick={() => setNotice("")}
              className="text-emerald-400 hover:text-white"
            >
              ✕
            </button>
          </div>
        )}

        {/* Header */}
        <div className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl border border-white/10 bg-white/5 text-white shadow-inner">
              <CheckSquare size={24} className="text-white" />
            </div>
            <div>
              <div className="flex items-center gap-3">
                <h1 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
                  Approval Center
                </h1>
                {pendingCount > 0 && (
                  <span className="inline-flex items-center gap-1.5 rounded-full border border-amber-500/30 bg-amber-500/10 px-3 py-1 text-xs font-semibold text-amber-300">
                    <span className="h-1.5 w-1.5 rounded-full bg-amber-400 animate-pulse" />
                    {pendingCount} Pending
                  </span>
                )}
              </div>
              <p className="mt-1 text-sm text-neutral-400">
                Review and verify accountability proofs submitted for your review
              </p>
            </div>
          </div>

          {/* Quick Refresh */}
          <button
            type="button"
            onClick={() => fetchInbox(activeTab, true)}
            disabled={refreshing || loading}
            className="inline-flex items-center justify-center gap-2 rounded-xl border border-white/10 bg-white/5 px-4 py-2 text-xs font-medium text-neutral-300 transition-all hover:bg-white/10 hover:text-white disabled:opacity-50"
          >
            <RefreshCw
              size={14}
              className={refreshing ? "animate-spin text-white" : ""}
            />
            {refreshing ? "Refreshing..." : "Refresh"}
          </button>
        </div>

        {/* Navigation Tabs and Search Bar */}
        <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          {/* Status Tabs */}
          <div className="flex overflow-x-auto rounded-xl border border-white/10 bg-white/[0.03] p-1">
            {TABS.map((tab) => {
              const isActive = activeTab === tab.id;
              const isPendingTab = tab.id === "pending";
              return (
                <button
                  key={tab.id}
                  onClick={() => setActiveTab(tab.id)}
                  className={`flex items-center gap-2 rounded-lg px-4 py-2 text-xs font-medium transition-all ${
                    isActive
                      ? "bg-white text-black shadow-sm font-semibold"
                      : "text-neutral-400 hover:text-white hover:bg-white/5"
                  }`}
                >
                  <span>{tab.label}</span>
                  {isPendingTab && pendingCount > 0 && (
                    <span
                      className={`rounded-full px-1.5 py-0.2 text-[10px] font-bold ${
                        isActive
                          ? "bg-black text-white"
                          : "bg-amber-500/20 text-amber-400 border border-amber-500/30"
                      }`}
                    >
                      {pendingCount}
                    </span>
                  )}
                </button>
              );
            })}
          </div>

          {/* Search Input */}
          <div className="relative sm:w-64">
            <Search
              size={14}
              className="absolute left-3 top-1/2 -translate-y-1/2 text-neutral-500"
            />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Filter by title, owner..."
              className="w-full rounded-xl border border-white/10 bg-white/[0.03] py-2 pl-9 pr-3 text-xs text-white placeholder-neutral-500 focus:border-white/30 focus:outline-none focus:ring-1 focus:ring-white/20"
            />
          </div>
        </div>

        {/* Content Section */}
        {loading ? (
          <div className="space-y-4">
            {[1, 2].map((n) => (
              <div
                key={n}
                className="animate-pulse rounded-2xl border border-white/5 bg-white/[0.02] p-6 space-y-4"
              >
                <div className="flex items-center gap-3">
                  <div className="h-11 w-11 rounded-xl bg-white/10" />
                  <div className="space-y-2">
                    <div className="h-4 w-32 rounded bg-white/10" />
                    <div className="h-3 w-20 rounded bg-white/5" />
                  </div>
                </div>
                <div className="h-5 w-3/4 rounded bg-white/10" />
                <div className="h-16 rounded-xl bg-white/5" />
              </div>
            ))}
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center gap-3 rounded-2xl border border-red-500/20 bg-red-500/5 p-8 text-center">
            <AlertCircle className="h-8 w-8 text-red-400" />
            <p className="text-sm font-medium text-red-300">{error}</p>
            <button
              onClick={() => fetchInbox(activeTab)}
              className="mt-2 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-2 text-xs font-medium text-red-200 transition-colors hover:bg-red-500/20"
            >
              Try Again
            </button>
          </div>
        ) : filteredItems.length === 0 ? (
          <div className="flex flex-col items-center justify-center rounded-2xl border border-white/10 bg-[#121217] p-12 text-center">
            {searchQuery.trim() ? (
              <>
                <Search className="mb-3 h-10 w-10 text-neutral-500" />
                <h3 className="text-base font-semibold text-white">
                  No matching goals found
                </h3>
                <p className="mt-1 max-w-sm text-xs text-neutral-400">
                  No goals match "{searchQuery}". Try a different keyword or clear the search.
                </p>
                <button
                  onClick={() => setSearchQuery("")}
                  className="mt-4 rounded-xl border border-white/10 bg-white/5 px-4 py-1.5 text-xs font-medium text-neutral-300 hover:text-white"
                >
                  Clear Search
                </button>
              </>
            ) : activeTab === "pending" ? (
              <>
                <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl border border-emerald-500/20 bg-emerald-500/10 text-emerald-400">
                  <CheckCircle2 size={28} />
                </div>
                <h3 className="text-lg font-semibold text-white">
                  All caught up!
                </h3>
                <p className="mt-1 max-w-sm text-xs leading-relaxed text-neutral-400">
                  You have no pending verification proofs waiting for your review. When an accountability partner submits proof, it will appear here instantly.
                </p>
              </>
            ) : activeTab === "approved" ? (
              <>
                <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl border border-white/10 bg-white/5 text-neutral-400">
                  <Shield size={28} />
                </div>
                <h3 className="text-lg font-semibold text-white">
                  No approved goals
                </h3>
                <p className="mt-1 max-w-sm text-xs leading-relaxed text-neutral-400">
                  Goals you review and approve will be archived here for your accountability records.
                </p>
              </>
            ) : activeTab === "rejected" ? (
              <>
                <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl border border-white/10 bg-white/5 text-neutral-400">
                  <XCircle size={28} />
                </div>
                <h3 className="text-lg font-semibold text-white">
                  No rejected proofs
                </h3>
                <p className="mt-1 max-w-sm text-xs leading-relaxed text-neutral-400">
                  Any proof submissions you reject with feedback will be shown here.
                </p>
              </>
            ) : (
              <>
                <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl border border-white/10 bg-white/5 text-neutral-400">
                  <Target size={28} />
                </div>
                <h3 className="text-lg font-semibold text-white">
                  No goals assigned
                </h3>
                <p className="mt-1 max-w-sm text-xs leading-relaxed text-neutral-400">
                  You haven't been assigned as an accountability partner for any goals yet.
                </p>
              </>
            )}
          </div>
        ) : (
          <div className="space-y-4">
            {filteredItems.map((item) => (
              <ApprovalCard
                key={item.goalId}
                item={item}
                onInspectPhoto={handleInspectPhoto}
                onApprove={handleApprove}
                onReject={handleReject}
              />
            ))}
          </div>
        )}

        {/* Evidence Inspection Lightbox Modal */}
        <EvidenceLightbox
          isOpen={Boolean(lightboxData)}
          onClose={() => setLightboxData(null)}
          item={lightboxData?.item}
          proof={lightboxData?.proof}
          onApprove={handleApprove}
          onReject={handleReject}
        />
      </div>
    </AppLayout>
  );
}
