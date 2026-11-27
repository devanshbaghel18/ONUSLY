import { Laptop, Globe, Smartphone, CheckCircle2 } from "lucide-react";
import lockAssembly from "../../assets/lock-assembly.webp";

export default function LandingEnforcers() {
  return (
    <section className="relative border-t border-[#26313d] bg-[#0b0e12] px-6 py-20 lg:px-16" id="enforcers">
      <div className="mx-auto max-w-7xl">
        {/* Section Header */}
        <div className="flex flex-col items-start justify-between gap-6 border-b border-[#26313d] pb-10 lg:flex-row lg:items-end">
          <div>
            <div className="inline-flex items-center gap-2 font-mono text-[11px] font-bold uppercase tracking-wider text-[#ff7843]">
              <span className="h-2 w-2 rounded-full bg-[#ff7843] animate-pulse" />
              <span>Total Environment Lock // How It Works</span>
            </div>
            <h2 className="mt-3 font-sans text-4xl font-extrabold tracking-tight text-white sm:text-5xl lg:text-6xl">
              Locks your devices.<br />
              <span className="text-[#ff7843]">Until the work is done.</span>
            </h2>
          </div>
          <p className="max-w-md text-sm text-[#8c9ba8] leading-relaxed">
            Willpower runs out by 3 PM. ONUSLY locks the apps, games, and websites that steal your focus so you don&apos;t have to fight yourself.
          </p>
        </div>

        {/* Layout: Visual Showcase + 3 Practical Cards */}
        <div className="mt-12 grid grid-cols-1 items-stretch gap-8 lg:grid-cols-12">
          {/* Left Column: Photorealistic Billet Assembly */}
          <div className="relative flex flex-col justify-between overflow-hidden rounded-lg border border-[#2b3542] bg-[#12161c] p-6 lg:col-span-5 shadow-2xl">
            {/* Header */}
            <div className="flex items-center justify-between border-b border-[#26313d] pb-3 font-mono text-[10px] text-neutral-400">
              <span className="font-bold uppercase tracking-wider text-white">THE COMMITMENT LOCK</span>
              <span className="text-[#10b981] font-bold">[READY TO LOCK]</span>
            </div>

            {/* Machined Image Preview */}
            <div className="relative my-6 flex items-center justify-center">
              <div className="absolute inset-0 bg-radial-gradient from-[#ff7843]/10 to-transparent blur-2xl" />
              <img
                src={lockAssembly}
                alt="ONUSLY Commitment Lock Assembly"
                className="relative z-10 w-full max-w-[340px] drop-shadow-[0_20px_50px_rgba(0,0,0,0.9)] transition-transform duration-500 hover:scale-105"
                fetchPriority="high"
              />
            </div>

            {/* Spec Matrix Footer */}
            <div className="space-y-2 border-t border-[#26313d] pt-4 font-mono text-[10px]">
              <div className="flex justify-between text-neutral-400">
                <span>WHAT IT LOCKS</span>
                <span className="font-bold text-white">Apps, Games & Websites</span>
              </div>
              <div className="flex justify-between text-neutral-400">
                <span>EMERGENCY BYPASS</span>
                <span className="font-bold text-[#ff7843]">None (No Passwords to Cheat With)</span>
              </div>
              <div className="flex justify-between text-neutral-400">
                <span>WHO UNLOCKS</span>
                <span className="font-bold text-[#10b981]">Your Chosen Witness</span>
              </div>
            </div>
          </div>

          {/* Right Column: 3 Plain-English Platform Cards */}
          <div className="grid grid-cols-1 gap-4 lg:col-span-7">
            {/* Card 1: On Your Computer */}
            <div className="group rounded-lg border border-[#26313d] bg-[#12161c] p-6 transition hover:border-[#384654] hover:bg-[#161c23]">
              <div className="flex items-center justify-between">
                <div className="flex h-10 w-10 items-center justify-center rounded border border-[#2d3744] bg-[#182028] text-[#ff7843]">
                  <Laptop size={20} />
                </div>
                <span className="rounded border border-[#ff7843]/40 bg-[#ff7843]/15 px-2.5 py-0.5 font-mono text-[10px] font-bold uppercase tracking-wider text-[#ff9769]">
                  On Your Computer
                </span>
              </div>
              <h3 className="mt-4 text-lg font-bold text-white">Windows & Linux</h3>
              <p className="mt-2 text-xs text-[#8c9ba8] leading-relaxed">
                If you try to launch Steam, Discord, or PC games in a moment of distraction, ONUSLY immediately closes the window before you can get sucked in.
              </p>
              <div className="mt-3 flex items-center gap-2 rounded border border-[#26313d] bg-[#0c0f14] px-3 py-2 font-mono text-[11px] text-neutral-300">
                <CheckCircle2 size={13} className="text-[#10b981] shrink-0" />
                <span>Also keeps distraction domains blocked across all web browsers on your machine.</span>
              </div>
            </div>

            {/* Card 2: In Your Browser */}
            <div className="group rounded-lg border border-[#26313d] bg-[#12161c] p-6 transition hover:border-[#384654] hover:bg-[#161c23]">
              <div className="flex items-center justify-between">
                <div className="flex h-10 w-10 items-center justify-center rounded border border-[#2d3744] bg-[#182028] text-[#ff7843]">
                  <Globe size={20} />
                </div>
                <span className="rounded border border-[#26313d] bg-[#161c23] px-2.5 py-0.5 font-mono text-[10px] font-bold uppercase tracking-wider text-neutral-300">
                  In Your Browser
                </span>
              </div>
              <h3 className="mt-4 text-lg font-bold text-white">Chrome, Brave, Edge & Firefox</h3>
              <p className="mt-2 text-xs text-[#8c9ba8] leading-relaxed">
                Stops the feeds that hijack your day. Smart enough to block YouTube Shorts while still letting you watch full coding, lecture, and tutorial videos.
              </p>
              <div className="mt-3 flex items-center gap-2 rounded border border-[#26313d] bg-[#0c0f14] px-3 py-2 font-mono text-[11px] text-neutral-300">
                <CheckCircle2 size={13} className="text-[#10b981] shrink-0" />
                <span>Zero lag, completely lightweight, and respects your privacy.</span>
              </div>
            </div>

            {/* Card 3: On Your Phone */}
            <div className="group rounded-lg border border-[#26313d] bg-[#12161c] p-6 transition hover:border-[#384654] hover:bg-[#161c23]">
              <div className="flex items-center justify-between">
                <div className="flex h-10 w-10 items-center justify-center rounded border border-[#2d3744] bg-[#182028] text-[#ff7843]">
                  <Smartphone size={20} />
                </div>
                <span className="rounded border border-[#26313d] bg-[#161c23] px-2.5 py-0.5 font-mono text-[10px] font-bold uppercase tracking-wider text-neutral-300">
                  On Your Phone
                </span>
              </div>
              <h3 className="mt-4 text-lg font-bold text-white">Mobile Apps & Social Feeds</h3>
              <p className="mt-2 text-xs text-[#8c9ba8] leading-relaxed">
                Puts social apps and mobile games out of reach at the exact same moment. One commitment locks your laptop, desktop, and phone together.
              </p>
              <div className="mt-3 flex items-center gap-2 rounded border border-[#26313d] bg-[#0c0f14] px-3 py-2 font-mono text-[11px] text-neutral-300">
                <CheckCircle2 size={13} className="text-[#10b981] shrink-0" />
                <span>When your friend approves your work, all your devices unlock simultaneously.</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
