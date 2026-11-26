import { useState } from "react";
import { ChevronDown, HelpCircle } from "lucide-react";

const faqItems = [
  {
    id: "friend-reply",
    q: "What if my friend is busy or takes a while to reply?",
    a: "We recommend picking people you talk to regularly—a roommate, study partner, or teammate—and agreeing on check-in times in advance (e.g. 'check my draft around 6 PM'). Because you can't cheat the lock, you'll naturally pick realistic commitments that matter.",
  },
  {
    id: "restart-cheat",
    q: "Can't I just restart my computer to bypass the lock?",
    a: "No. The lock is built to protect you during moments of fatigue. Distracting websites and desktop apps stay locked even across system reboots until your witness sends the approval.",
  },
  {
    id: "youtube-shorts",
    q: "Can I really block YouTube Shorts without blocking full YouTube?",
    a: "Yes! That is one of ONUSLY's most popular features. You can watch long-form tutorials, lectures, and documentaries without ever seeing or getting pulled into the addictive Shorts feed.",
  },
  {
    id: "offline-internet",
    q: "What happens if I lose internet connection while working?",
    a: "Your apps and sites stay locked even without an internet connection. Once your Wi-Fi is back, your computer checks for your partner's approval and unlocks immediately.",
  },
  {
    id: "proof-types",
    q: "What kind of proof can I submit to my witness?",
    a: "Anything that shows you did the work: a photo of your desk or notebook, a PDF summary, a code link, or a clear screenshot of your completed milestone.",
  },
  {
    id: "free-to-start",
    q: "Is it free to start?",
    a: "Yes. You can create a commitment and start locking distractions with an accountability partner right away.",
  },
];

export default function LandingFAQ() {
  const [openIds, setOpenIds] = useState(new Set(["friend-reply"]));

  const toggle = (id) => {
    setOpenIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  return (
    <section className="relative border-t border-[#26313d] bg-[#0b0e12] px-6 py-20 lg:px-16" id="faq">
      <div className="mx-auto max-w-5xl">
        {/* Header */}
        <div className="text-center">
          <div className="inline-flex items-center gap-2 font-mono text-[11px] font-bold uppercase tracking-wider text-[#ff7843]">
            <HelpCircle size={14} />
            <span>Common Questions</span>
          </div>
          <h2 className="mt-3 font-sans text-4xl font-extrabold tracking-tight text-white sm:text-5xl">
            Frequently Asked Questions.
          </h2>
          <p className="mt-2 text-sm text-[#8c9ba8]">
            Everything you need to know about locks, witnesses, and real focus.
          </p>
        </div>

        {/* Accordion List */}
        <div className="mt-12 space-y-3">
          {faqItems.map((item) => {
            const isOpen = openIds.has(item.id);
            return (
              <div
                key={item.id}
                className="rounded-lg border border-[#26313d] bg-[#12161c] transition overflow-hidden"
              >
                <button
                  type="button"
                  onClick={() => toggle(item.id)}
                  className="flex w-full items-center justify-between p-5 text-left transition hover:bg-[#161c23]"
                >
                  <span className="text-sm font-bold text-white tracking-tight pr-4">
                    {item.q}
                  </span>
                  <div className={`flex h-6 w-6 shrink-0 items-center justify-center rounded border border-[#26313d] bg-[#0c0f14] text-neutral-400 transition-transform duration-200 ${isOpen ? "rotate-180 text-[#ff7843]" : ""}`}>
                    <ChevronDown size={14} />
                  </div>
                </button>

                {isOpen && (
                  <div className="border-t border-[#26313d] bg-[#0e1216] px-5 py-4 text-xs text-[#8c9ba8] leading-relaxed">
                    {item.a}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}
