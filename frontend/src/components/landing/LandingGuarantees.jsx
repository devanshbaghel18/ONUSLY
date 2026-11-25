import { Shield, Lock, FileCheck2, HardDrive, CheckCircle2 } from "lucide-react";

const guarantees = [
  {
    tag: "RULE 01 // NO ESCAPE HATCH",
    title: "No Emergency Override Passwords",
    desc: "Blockers with passwords fail because in a moment of weakness, you always type the password. ONUSLY gives the unlock key to someone else.",
    icon: Lock,
  },
  {
    tag: "RULE 02 // HUMAN WITNESS",
    title: "You Can't Approve Yourself",
    desc: "Self-approval is blocked. You cannot simply check a box to lift the restrictions. Your accountability partner must inspect your work.",
    icon: Shield,
  },
  {
    tag: "RULE 03 // ALWAYS ON",
    title: "Can't Just Restart or Unplug Wi-Fi",
    desc: "Restarting your computer, closing background windows, or disconnecting your internet will not release the lock. The apps stay locked until approved.",
    icon: HardDrive,
  },
  {
    tag: "RULE 04 // REAL RESULTS",
    title: "Proof You Can Be Proud Of",
    desc: "Every completed goal builds an undeniable receipt showing what you accomplished, the evidence you submitted, and who verified it.",
    icon: FileCheck2,
  },
];

export default function LandingGuarantees() {
  return (
    <section className="relative border-t border-[#26313d] bg-[#0e1216] px-6 py-20 lg:px-16" id="guarantees">
      <div className="mx-auto max-w-7xl">
        {/* Header */}
        <div className="flex flex-col items-start justify-between gap-6 border-b border-[#26313d] pb-10 lg:flex-row lg:items-end">
          <div>
            <div className="inline-flex items-center gap-2 font-mono text-[11px] font-bold uppercase tracking-wider text-[#ff7843]">
              <span className="h-2 w-2 rounded-full bg-[#ff7843] animate-pulse" />
              <span>Unbreakable Promises // Hard Guarantees</span>
            </div>
            <h2 className="mt-3 font-sans text-4xl font-extrabold tracking-tight text-white sm:text-5xl lg:text-6xl">
              Built for the moments<br />
              <span className="text-[#ff7843]">you want to give up.</span>
            </h2>
          </div>
          <p className="max-w-md text-sm text-[#8c9ba8] leading-relaxed">
            Most focus tools fail because they assume you&apos;ll be rational when you&apos;re tired.
            ONUSLY is designed to protect your commitments when your discipline wavers.
          </p>
        </div>

        {/* 4 Cards Grid */}
        <div className="mt-12 grid grid-cols-1 gap-6 md:grid-cols-2">
          {guarantees.map((g) => {
            const Icon = g.icon;
            return (
              <div
                key={g.tag}
                className="relative flex flex-col justify-between rounded-lg border border-[#26313d] bg-[#12161c] p-6 lg:p-7 transition hover:border-[#384654] hover:bg-[#161c23]"
              >
                <div>
                  <div className="flex items-center justify-between border-b border-[#26313d] pb-3">
                    <span className="font-mono text-[10px] font-bold uppercase tracking-wider text-[#ff7843]">
                      {g.tag}
                    </span>
                    <div className="flex h-8 w-8 items-center justify-center rounded border border-[#2d3744] bg-[#182028] text-[#ff7843]">
                      <Icon size={16} />
                    </div>
                  </div>

                  <h3 className="mt-4 text-lg font-bold text-white tracking-tight">
                    {g.title}
                  </h3>
                  <p className="mt-2 text-xs text-[#8c9ba8] leading-relaxed">
                    {g.desc}
                  </p>
                </div>

                <div className="mt-6 flex items-center gap-2 font-mono text-[10px] text-[#10b981]">
                  <CheckCircle2 size={12} />
                  <span>PERMANENTLY ENFORCED</span>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}
