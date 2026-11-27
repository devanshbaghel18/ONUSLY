import { Link } from "react-router-dom";
import Brand from "../components/Brand";

export default function Legal({ mode = "privacy" }) {
  const isPrivacy = mode === "privacy";

  return (
    <div className="min-h-screen bg-[#0c0e11] text-[#e8e4dc] px-6 py-10">
      <div className="mx-auto max-w-3xl">
        <header className="flex items-center justify-between border-b border-[#26313d] pb-6">
          <Link to="/" aria-label="ONUSLY home">
            <Brand />
          </Link>
          <div className="flex items-center gap-4 font-mono text-xs text-neutral-400">
            <Link to="/privacy" className={isPrivacy ? "text-[#ff7843] font-bold" : "hover:text-white"}>
              Privacy
            </Link>
            <Link to="/terms" className={!isPrivacy ? "text-[#ff7843] font-bold" : "hover:text-white"}>
              Terms
            </Link>
            <Link to="/" className="rounded border border-[#26313d] px-3 py-1.5 hover:text-white">
              Back Home
            </Link>
          </div>
        </header>

        <main className="mt-10 space-y-6">
          <div className="font-mono text-[11px] uppercase tracking-wider text-[#ff7843]">
            ONUSLY // {isPrivacy ? "Privacy Policy" : "Terms of Service"}
          </div>
          <h1 className="text-3xl font-extrabold tracking-tight text-white">
            {isPrivacy ? "Privacy Policy" : "Terms of Service"}
          </h1>

          {isPrivacy ? (
            <div className="space-y-4 text-sm leading-relaxed text-neutral-300">
              <p>
                ONUSLY collects only the account identifiers (Google subject ID, email address, display name, and handle), commitment rules, device telemetry heartbeats, and uploaded verification proof files required to operate accountability enforcement and partner approvals.
              </p>
              <p>
                Proof media uploaded for a commitment is stored privately on the ONUSLY API server and restricted strictly to the goal owner and the goal&apos;s designated accountability partner. ONUSLY never sells user data or third-party browsing histories.
              </p>
              <p>
                Browser extension and desktop agent enforcement evaluate distracting domains and application process names locally using the active signed-in user&apos;s synchronized commitment policy. You can revoke device sessions or sign out after completing active commitments.
              </p>
            </div>
          ) : (
            <div className="space-y-4 text-sm leading-relaxed text-neutral-300">
              <p>
                ONUSLY is an accountability commitment tool designed to enforce self-selected distraction locks across your paired desktop, browser, and mobile devices until your chosen accountability partner verifies submitted proof.
              </p>
              <p>
                Commitments containing blocked targets require partner verification before unlock. You are responsible for assigning a trusted accountability partner and submitting accurate completion evidence without violating third-party rights or uploading malicious content.
              </p>
              <p>
                Administrative or system-level DNS and application enforcement software should only be installed on devices you own or are authorized to administer.
              </p>
            </div>
          )}
        </main>
      </div>
    </div>
  );
}
