import { useState } from "react";
import { Lock, Sparkles, ShieldCheck } from "lucide-react";
import { APP_CATALOG } from "../../lib/blockedApps";

const categories = [
  { id: "all", label: "All Apps & Sites" },
  { id: "social", label: "Social Feeds" },
  { id: "media", label: "Video & Streaming" },
  { id: "gaming", label: "Gaming & Chat" },
];

const appDescriptions = {
  youtube: "Keep long tutorials open while blocking doomscroll feeds.",
  youtube_shorts: "Isolates and blocks endless Shorts while keeping full lectures.",
  instagram: "Locks Reels, Stories, DMs, and Explore feeds.",
  twitter: "Silences trending topics, notifications, and endless replies.",
  reddit: "Blocks home feeds and endless comment threads.",
  netflix: "Shuts down background movie and show streaming.",
  discord: "Mutes community chats, servers, and group calls.",
  twitch: "Locks live game streams and chat rooms.",
  tiktok: "Puts the infinite video algorithm completely out of reach.",
  facebook: "Locks news feeds, watch tabs, and notifications.",
  snapchat: "Blocks mobile stories, spotlight, and messaging.",
  linkedin: "Locks infinite professional feed scrolling.",
  pinterest: "Blocks endless visual idea feeds.",
  steam: "Keeps your game library locked until your daily work is in.",
  spotify: "Prevents endless podcast browsing during focus blocks.",
  slack: "Silences non-urgent workspace pings during deep work.",
  telegram: "Locks public channels and chat groups.",
  vlc: "Blocks local video and media playback.",
  obs: "Locks recording and streaming suites until setup is verified.",
};

export default function LandingCatalog({ onSignIn }) {
  const [activeCategory, setActiveCategory] = useState("all");

  const filteredApps = APP_CATALOG.filter((app) => {
    if (activeCategory === "all") return true;
    if (activeCategory === "social") {
      return ["instagram", "twitter", "reddit", "tiktok", "facebook", "pinterest", "linkedin"].includes(app.id);
    }
    if (activeCategory === "media") {
      return ["youtube", "youtube_shorts", "netflix", "spotify"].includes(app.id);
    }
    if (activeCategory === "gaming") {
      return ["steam", "discord", "twitch", "telegram", "obs"].includes(app.id);
    }
    return true;
  });

  return (
    <section className="relative border-t border-[#26313d] bg-[#0b0e12] px-6 py-20 lg:px-16" id="catalog">
      <div className="mx-auto max-w-7xl">
        {/* Header */}
        <div className="flex flex-col items-start justify-between gap-6 border-b border-[#26313d] pb-10 lg:flex-row lg:items-end">
          <div>
            <div className="inline-flex items-center gap-2 font-mono text-[11px] font-bold uppercase tracking-wider text-[#ff7843]">
              <span className="h-2 w-2 rounded-full bg-[#ff7843] animate-pulse" />
              <span>Coverage Catalog // Distraction Defense</span>
            </div>
            <h2 className="mt-3 font-sans text-4xl font-extrabold tracking-tight text-white sm:text-5xl lg:text-6xl">
              Lock the apps you<br />
              <span className="text-[#ff7843]">lose hours to.</span>
            </h2>
          </div>
          <p className="max-w-md text-sm text-[#8c9ba8] leading-relaxed">
            Ready-to-lock presets for today&apos;s most addictive social media feeds, video algorithms, and gaming platforms.
          </p>
        </div>

        {/* Category Filter Tabs */}
        <div className="mt-8 flex flex-wrap items-center gap-2">
          {categories.map((cat) => (
            <button
              key={cat.id}
              type="button"
              onClick={() => setActiveCategory(cat.id)}
              className={`rounded border px-3.5 py-1.5 font-mono text-xs font-bold uppercase tracking-wider transition ${
                activeCategory === cat.id
                  ? "border-[#ff7843] bg-[#1c2430] text-white shadow-sm"
                  : "border-[#26313d] bg-[#12161c] text-neutral-400 hover:border-[#384654] hover:text-white"
              }`}
            >
              {cat.label}
            </button>
          ))}
        </div>

        {/* Target Cards Grid */}
        <div className="mt-8 grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4">
          {filteredApps.map((item) => {
            const isShorts = item.id === "youtube_shorts";
            const desc = appDescriptions[item.id] || "Locks app and website access across devices.";
            return (
              <div
                key={item.id}
                className="group relative flex flex-col justify-between rounded-lg border border-[#26313d] bg-[#12161c] p-4 transition hover:border-[#ff7843]/60 hover:bg-[#161c23]"
              >
                <div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-bold text-white group-hover:text-[#ff7843] transition">
                      {item.name}
                    </span>
                    <Lock size={13} className="text-neutral-500 group-hover:text-[#ff7843] transition" />
                  </div>

                  {isShorts && (
                    <div className="mt-2 inline-flex items-center gap-1 rounded border border-[#ff7843]/40 bg-[#ff7843]/15 px-2 py-0.5 font-mono text-[9px] font-bold text-[#ff9769]">
                      <Sparkles size={9} />
                      <span>Smart Shorts Filter</span>
                    </div>
                  )}

                  <p className="mt-2 text-xs text-[#8c9ba8] leading-relaxed">
                    {desc}
                  </p>
                </div>

                <div className="mt-4 flex items-center justify-between border-t border-[#26313d] pt-2 font-mono text-[9px] text-neutral-400">
                  <span className="text-[#10b981]">LOCKABLE</span>
                  <span>COMPUTER & PHONE</span>
                </div>
              </div>
            );
          })}
        </div>

        {/* Custom Target Callout Box */}
        <div className="mt-8 rounded-lg border border-[#2b3542] bg-[#12161c] p-6 lg:p-8">
          <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-6">
            <div className="space-y-2">
              <div className="inline-flex items-center gap-2 font-mono text-[10px] font-bold uppercase tracking-wider text-[#ff7843]">
                <ShieldCheck size={14} />
                <span>Custom Targets</span>
              </div>
              <h3 className="text-xl font-bold text-white">Have a specific weakness or game you play?</h3>
              <p className="max-w-2xl text-xs text-[#8c9ba8] leading-relaxed">
                You aren&apos;t limited to our catalog. Type in any application on your computer (like games, video editors, or players) or any website URL. ONUSLY locks it with the exact same unyielding rules.
              </p>
            </div>
            <button
              type="button"
              onClick={onSignIn}
              className="rounded border border-[#384654] bg-[#1c2430] px-5 py-2.5 font-mono text-xs font-bold uppercase tracking-wider text-white hover:border-[#ff7843] hover:text-[#ff7843] transition shrink-0"
            >
              Lock a Custom Target
            </button>
          </div>
        </div>
      </div>
    </section>
  );
}
