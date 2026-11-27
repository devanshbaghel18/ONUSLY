import { Download, ArrowUpRight } from "lucide-react";
import Brand from "../Brand";

export default function LandingFooter({ onSignIn }) {
  return (
    <footer className="relative border-t border-[#26313d] bg-[#080b0e] px-6 pt-20 pb-12 lg:px-16" id="downloads">
      <div className="mx-auto max-w-7xl">
        {/* Call to Action Billet */}
        <div className="relative overflow-hidden rounded-xl border border-[#2b3542] bg-[#12161c] p-8 lg:p-12 shadow-2xl text-center">
          <div className="mx-auto max-w-2xl space-y-4">
            <div className="inline-flex items-center gap-2 rounded border border-[#ff7843]/40 bg-[#ff7843]/15 px-3 py-1 font-mono text-[10px] font-bold uppercase tracking-wider text-[#ff9769]">
              <span className="h-1.5 w-1.5 rounded-full bg-[#ff7843] animate-pulse" />
              <span>Real Accountability</span>
            </div>
            <h2 className="font-sans text-3xl font-extrabold tracking-tight text-white sm:text-5xl">
              Lock the distractions.<br />
              <span className="text-[#ff7843]">Finish what you started.</span>
            </h2>
            <p className="text-sm text-[#8c9ba8] leading-relaxed">
              Start with a single commitment. Lock distracting apps and assign your witness.
              Free to use with your study partners, friends, and teammates.
            </p>
            <div className="pt-4 flex flex-wrap items-center justify-center gap-3">
              <button
                type="button"
                onClick={onSignIn}
                className="flex items-center gap-2 rounded border border-[#ff7843] bg-[#ff7843] px-6 py-3 font-mono text-xs font-bold uppercase tracking-wider text-black shadow-lg hover:bg-[#ff8f5e] transition active:scale-95"
              >
                <span>Create Your First Lock</span>
                <ArrowUpRight size={15} />
              </button>
            </div>
          </div>
        </div>

        {/* Client Downloads Grid */}
        <div className="mt-16 border-t border-[#26313d] pt-12">
          <div className="flex items-center justify-between mb-6">
            <h3 className="font-mono text-xs font-bold uppercase tracking-wider text-neutral-400">
              Apps & Extensions
            </h3>
            <span className="font-mono text-[10px] text-neutral-500">v1.0.0 READY</span>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-5">
            {/* Windows Desktop */}
            <a
              href="/downloads/OnuslySetup.exe"
              download="OnuslySetup.exe"
              className="group flex flex-col justify-between rounded-lg border border-[#26313d] bg-[#12161c] p-4 transition hover:border-[#384654] hover:bg-[#161c23]"
            >
              <div>
                <div className="flex items-center justify-between text-neutral-400 group-hover:text-white transition">
                  <span className="font-mono text-[10px] font-bold uppercase">Windows App</span>
                  <Download size={14} />
                </div>
                <div className="mt-2 text-sm font-bold text-white">Onusly for Windows</div>
                <p className="mt-1 text-[11px] text-[#8c9ba8]">Locks desktop apps, Steam games, and distractions on your PC.</p>
              </div>
              <span className="mt-4 font-mono text-[10px] text-[#ff7843]">Download .exe →</span>
            </a>

            {/* Chromium Extension */}
            <a
              href="/downloads/onusly-extension.zip"
              download="onusly-extension.zip"
              className="group flex flex-col justify-between rounded-lg border border-[#26313d] bg-[#12161c] p-4 transition hover:border-[#384654] hover:bg-[#161c23]"
            >
              <div>
                <div className="flex items-center justify-between text-neutral-400 group-hover:text-white transition">
                  <span className="font-mono text-[10px] font-bold uppercase">Chrome / Brave / Edge</span>
                  <Download size={14} />
                </div>
                <div className="mt-2 text-sm font-bold text-white">Browser Extension</div>
                <p className="mt-1 text-[11px] text-[#8c9ba8]">Locks distracting websites and YouTube Shorts in your browser.</p>
              </div>
              <span className="mt-4 font-mono text-[10px] text-[#ff7843]">Download Zip →</span>
            </a>

            {/* Firefox Extension */}
            <a
              href="/downloads/onusly-firefox.zip"
              download="onusly-firefox.zip"
              className="group flex flex-col justify-between rounded-lg border border-[#26313d] bg-[#12161c] p-4 transition hover:border-[#384654] hover:bg-[#161c23]"
            >
              <div>
                <div className="flex items-center justify-between text-neutral-400 group-hover:text-white transition">
                  <span className="font-mono text-[10px] font-bold uppercase">Firefox</span>
                  <Download size={14} />
                </div>
                <div className="mt-2 text-sm font-bold text-white">Firefox Add-on</div>
                <p className="mt-1 text-[11px] text-[#8c9ba8]">Full website and Shorts lock protection for Firefox.</p>
              </div>
              <span className="mt-4 font-mono text-[10px] text-[#ff7843]">Download Zip →</span>
            </a>

            {/* 1-Click DNS Switcher */}
            <a
              href="/downloads/ONUSLY_Shield_Toggle.exe"
              download="ONUSLY_Shield_Toggle.exe"
              className="group flex flex-col justify-between rounded-lg border border-[#26313d] bg-[#12161c] p-4 transition hover:border-[#384654] hover:bg-[#161c23]"
            >
              <div>
                <div className="flex items-center justify-between text-neutral-400 group-hover:text-white transition">
                  <span className="font-mono text-[10px] font-bold uppercase">Quick Tool</span>
                  <Download size={14} />
                </div>
                <div className="mt-2 text-sm font-bold text-white">1-Click DNS Shield</div>
                <p className="mt-1 text-[11px] text-[#8c9ba8]">Quick Windows desktop toggle to block domains system-wide.</p>
              </div>
              <span className="mt-4 font-mono text-[10px] text-[#ff7843]">Download Tool →</span>
            </a>

            {/* Android Companion APK */}
            <a
              href="/downloads/onusly.apk"
              download="onusly.apk"
              className="group flex flex-col justify-between rounded-lg border border-[#26313d] bg-[#12161c] p-4 transition hover:border-[#384654] hover:bg-[#161c23]"
            >
              <div>
                <div className="flex items-center justify-between text-neutral-400 group-hover:text-white transition">
                  <span className="font-mono text-[10px] font-bold uppercase">Android</span>
                  <Download size={14} />
                </div>
                <div className="mt-2 text-sm font-bold text-white">Android App</div>
                <p className="mt-1 text-[11px] text-[#8c9ba8]">Locks mobile apps, reels, and domains alongside your PC.</p>
              </div>
              <span className="mt-4 font-mono text-[10px] text-[#ff7843]">Download .apk →</span>
            </a>
          </div>
        </div>

        {/* System Status & Colophon */}
        <div className="mt-16 flex flex-col items-center justify-between gap-6 border-t border-[#26313d] pt-8 lg:flex-row">
          <div className="flex items-center gap-4">
            <Brand />
            <span className="h-4 w-[1px] bg-[#26313d]" />
            <div className="inline-flex items-center gap-1.5 font-mono text-[10px] text-[#10b981]">
              <span className="h-1.5 w-1.5 rounded-full bg-[#10b981] animate-pulse" />
              <span>SYSTEM READY // ALL ENFORCERS ONLINE</span>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-6 font-mono text-xs text-neutral-400">
            <a href="#enforcers" className="hover:text-white transition">How it works</a>
            <a href="#protocol" className="hover:text-white transition">Steps</a>
            <a href="#catalog" className="hover:text-white transition">Apps</a>
            <a href="#guarantees" className="hover:text-white transition">Guarantees</a>
            <a href="#faq" className="hover:text-white transition">FAQ</a>
            <a href="/privacy" className="hover:text-white transition">Privacy</a>
            <a href="/terms" className="hover:text-white transition">Terms</a>
            <button type="button" onClick={onSignIn} className="text-[#ff7843] hover:underline">
              Sign In
            </button>
          </div>
        </div>

        {/* Copyright notice */}
        <div className="mt-6 text-center text-[10px] font-mono text-neutral-600">
          © {new Date().getFullYear()} ONUSLY INSTRUMENTS. UNBREAKABLE ACCOUNTABILITY.
        </div>
      </div>
    </footer>
  );
}
