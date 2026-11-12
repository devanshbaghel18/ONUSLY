import React from "react";
import { AlertTriangle, RefreshCw } from "lucide-react";

export default class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error) {
    return { hasError: true, error };
  }

  componentDidCatch(error, errorInfo) {
    console.error("[ErrorBoundary] Uncaught render error:", error, errorInfo);
  }

  handleReload = () => {
    window.location.reload();
  };

  render() {
    if (this.state.hasError) {
      return (
        <div className="min-h-screen bg-[#0E0E12] text-white flex flex-col items-center justify-center p-6 text-center">
          <div className="max-w-md w-full rounded-2xl border border-white/10 bg-[#141418] p-8 shadow-2xl space-y-4">
            <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl border border-amber-500/20 bg-amber-500/10 text-amber-400">
              <AlertTriangle size={24} />
            </div>
            <h2 className="text-lg font-bold text-white">Something went wrong</h2>
            <p className="text-xs text-neutral-400 leading-relaxed">
              An unexpected display issue occurred while rendering the page.
            </p>
            {this.state.error?.message && (
              <div className="rounded-xl border border-white/10 bg-black/40 p-3 text-[11px] font-mono text-neutral-300 text-left overflow-x-auto">
                {this.state.error.message}
              </div>
            )}
            <div className="pt-2">
              <button
                type="button"
                onClick={this.handleReload}
                className="btn-glow inline-flex items-center gap-2 rounded-xl px-5 py-2.5 text-xs font-bold text-black"
              >
                <RefreshCw size={14} />
                <span>Reload Dashboard</span>
              </button>
            </div>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}

