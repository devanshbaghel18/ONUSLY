import { useState, useEffect } from "react";
import AppLayout from "../components/AppLayout";
import { CheckSquare, Loader2, AlertCircle } from "lucide-react";
import { getApproverInbox } from "../lib/api";

export default function ApprovalCenter() {
  const [inbox, setInbox] = useState({ items: [], total: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [statusFilter, setStatusFilter] = useState("pending");

  useEffect(() => {
    let mounted = true;
    setLoading(true);
    setError("");

    getApproverInbox(statusFilter)
      .then((data) => {
        if (mounted) {
          setInbox(data || { items: [], total: 0 });
          setLoading(false);
        }
      })
      .catch((err) => {
        if (mounted) {
          setError(err?.message || "Failed to load approval inbox");
          setLoading(false);
        }
      });

    return () => {
      mounted = false;
    };
  }, [statusFilter]);

  return (
    <AppLayout>
      <div className="mx-auto max-w-5xl px-4 py-8 sm:px-6 lg:px-8">
        {/* Header */}
        <div className="mb-8 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-white/10 bg-white/5 text-white">
                <CheckSquare size={20} />
              </div>
              <div>
                <h1 className="text-2xl font-bold tracking-tight text-white">
                  Approval Center
                </h1>
                <p className="text-sm text-neutral-400">
                  Review and verify accountability proofs submitted for your review
                </p>
              </div>
            </div>
          </div>
          {inbox.total > 0 && statusFilter === "pending" && (
            <div className="flex items-center gap-2 rounded-full border border-red-500/20 bg-red-500/10 px-3 py-1 text-xs font-semibold text-red-400">
              <span className="h-1.5 w-1.5 rounded-full bg-red-400 animate-pulse" />
              {inbox.total} pending {inbox.total === 1 ? "review" : "reviews"}
            </div>
          )}
        </div>

        {/* Content State */}
        {loading ? (
          <div className="flex h-64 flex-col items-center justify-center gap-3 rounded-2xl border border-white/10 bg-white/[0.02]">
            <Loader2 className="h-8 w-8 animate-spin text-neutral-400" />
            <p className="text-sm text-neutral-400">Loading inbox...</p>
          </div>
        ) : error ? (
          <div className="flex h-64 flex-col items-center justify-center gap-3 rounded-2xl border border-red-500/20 bg-red-500/5 p-6 text-center">
            <AlertCircle className="h-8 w-8 text-red-400" />
            <p className="text-sm text-red-300">{error}</p>
            <button
              onClick={() => setStatusFilter((s) => s)}
              className="mt-2 rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-1.5 text-xs font-medium text-red-200 transition-colors hover:bg-red-500/20"
            >
              Retry
            </button>
          </div>
        ) : (
          <div className="rounded-2xl border border-white/10 bg-[#121217] p-8 text-center text-neutral-400">
            <p>Inbox connected successfully. Total items: {inbox.total}</p>
          </div>
        )}
      </div>
    </AppLayout>
  );
}

