import { Lock, ShieldAlert, FileText, KeyRound, ArrowRight } from "lucide-react";

const steps = [
  {
    num: "01",
    tag: "START YOUR LOCK",
    title: "Pick Your Goal & Choose a Witness",
    desc: "Select the apps, games, or websites that derail you. Name your task and assign a friend, study partner, or teammate to be your witness.",
    icon: Lock,
    accent: "text-[#ff7843]",
  },
  {
    num: "02",
    tag: "ZERO DISTRACTIONS",
    title: "Stay in the Zone",
    desc: "No passwords to cheat with. No 'just 5 more minutes' button. Your chosen distractions stay firmly locked until you finish what you started.",
    icon: ShieldAlert,
    accent: "text-amber-400",
  },
  {
    num: "03",
    tag: "SUBMIT PROOF",
    title: "Show Your Work",
    desc: "Hit your milestone? Take a photo of your notebook or desk, attach a summary document, or share a link to what you built.",
    icon: FileText,
    accent: "text-[#ff7843]",
  },
  {
    num: "04",
    tag: "LIFT THE LOCK",
    title: "Your Witness Approves",
    desc: "Your partner gets an instant review alert, checks your evidence, and taps approve. The latch opens and your apps unlock in seconds.",
    icon: KeyRound,
    accent: "text-[#10b981]",
  },
];

export default function LandingProtocol({ onSignIn }) {
  return (
    <section className="relative border-t border-[#26313d] bg-[#0e1216] px-6 py-20 lg:px-16" id="protocol">
      <div className="mx-auto max-w-7xl">
        {/* Header */}
        <div className="flex flex-col items-start justify-between gap-6 border-b border-[#26313d] pb-10 lg:flex-row lg:items-end">
          <div>
            <div className="inline-flex items-center gap-2 font-mono text-[11px] font-bold uppercase tracking-wider text-[#ff7843]">
              <span className="h-2 w-2 rounded-full bg-[#ff7843] animate-pulse" />
              <span>The Accountability Flow // Step by Step</span>
            </div>
            <h2 className="mt-3 font-sans text-4xl font-extrabold tracking-tight text-white sm:text-5xl lg:text-6xl">
              Give the key to someone<br />
              <span className="text-[#ff7843]">who wants you to win.</span>
            </h2>
          </div>
          <p className="max-w-md text-sm text-[#8c9ba8] leading-relaxed">
            Every blocker that lets you type a password or hit &apos;pause&apos; fails the moment you get tired.
            ONUSLY transfers the unlock key to a friend who actually cares whether you finish.
          </p>
        </div>

        {/* 4 Steps Grid */}
        <div className="mt-12 grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-4">
          {steps.map((s, idx) => {
            const Icon = s.icon;
            return (
              <div
                key={s.num}
                className="relative flex flex-col justify-between rounded-lg border border-[#26313d] bg-[#12161c] p-6 transition hover:border-[#384654] hover:bg-[#161c23]"
              >
                <div>
                  {/* Step Top Bar */}
                  <div className="flex items-center justify-between border-b border-[#26313d] pb-3">
                    <span className="font-mono text-2xl font-black text-neutral-600">
                      {s.num}
                    </span>
                    <div className={`flex h-8 w-8 items-center justify-center rounded border border-[#2d3744] bg-[#182028] ${s.accent}`}>
                      <Icon size={16} />
                    </div>
                  </div>

                  {/* Stage Tag */}
                  <div className="mt-4">
                    <span className="rounded border border-[#26313d] bg-[#0c0f14] px-2 py-0.5 font-mono text-[9px] font-bold uppercase tracking-wider text-neutral-300">
                      {s.tag}
                    </span>
                  </div>

                  {/* Content */}
                  <h3 className="mt-3 text-base font-bold text-white tracking-tight">
                    {s.title}
                  </h3>
                  <p className="mt-2 text-xs text-[#8c9ba8] leading-relaxed">
                    {s.desc}
                  </p>
                </div>

                {/* Sub-status indicator */}
                <div className="mt-6 flex items-center justify-between border-t border-[#26313d] pt-3 font-mono text-[10px] text-neutral-400">
                  <span>STEP {idx + 1} OF 4</span>
                  <span className={s.accent}>UNBREAKABLE</span>
                </div>
              </div>
            );
          })}
        </div>

        {/* Banner Callout */}
        <div className="mt-12 flex flex-col items-center justify-between gap-6 rounded-lg border border-[#2b3542] bg-[#141920] p-6 lg:flex-row lg:p-8">
          <div className="space-y-1">
            <h4 className="text-base font-bold text-white">
              Ready to lock your first deep work session?
            </h4>
            <p className="text-xs text-[#8c9ba8]">
              Takes 30 seconds to set up. Free to use with any friend or accountability partner.
            </p>
          </div>
          <button
            type="button"
            onClick={onSignIn}
            className="flex items-center gap-2 rounded border border-[#ff7843] bg-[#ff7843] px-5 py-2.5 font-mono text-xs font-bold uppercase tracking-wider text-black shadow-lg hover:bg-[#ff8f5e] transition active:scale-95 shrink-0"
          >
            <span>Start a Commitment</span>
            <ArrowRight size={14} />
          </button>
        </div>
      </div>
    </section>
  );
}
