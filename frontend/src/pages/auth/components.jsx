import { useState } from "react";
import { Eye, EyeOff, Shield } from "lucide-react";

export function OnuslyLogo() {
  return (
    <div className="flex flex-col items-center">
      <div className="mb-3 flex h-14 w-14 items-center justify-center rounded-2xl border border-white/20 bg-gradient-to-b from-white/15 to-white/5 text-white shadow-[0_0_30px_rgba(255,255,255,0.12)]">
        <Shield size={28} className="stroke-[2.2]" />
      </div>
      <div className="flex items-center gap-2">
        <span className="font-heading text-3xl font-black tracking-tight text-white">
          ONUSLY
        </span>
        <span className="rounded-full border border-white/20 bg-white/10 px-2 py-0.5 font-mono text-[10px] font-bold uppercase tracking-wider text-neutral-300">
          v2.4
        </span>
      </div>
      <p className="mt-1 font-mono text-[11px] uppercase tracking-widest text-neutral-400">
        Accountability & Focus Lockout Engine
      </p>
    </div>
  );
}

export function Field({
  label,
  icon: Icon,
  type = "text",
  placeholder,
  autoComplete,
}) {
  const [showPassword, setShowPassword] = useState(false);
  const isPassword = type === "password";

  return (
    <div className="space-y-1.5">
      <label className="block text-[11px] font-bold uppercase tracking-wider text-neutral-300">
        {label}
      </label>
      <div className="relative group">
        <Icon
          size={16}
          className="absolute left-3.5 top-1/2 -translate-y-1/2 text-neutral-400 group-focus-within:text-white transition-colors"
        />
        <input
          type={isPassword && showPassword ? "text" : type}
          placeholder={placeholder}
          autoComplete={autoComplete}
          required
          className="w-full rounded-xl border border-white/10 bg-[#0E0E12] py-2.5 pl-10 pr-10 text-xs text-white placeholder:text-neutral-500 outline-none transition-all duration-200 focus:border-white/40 focus:bg-[#121216] focus:ring-1 focus:ring-white/30 hover:border-white/20"
        />
        {isPassword && (
          <button
            type="button"
            onClick={() => setShowPassword((value) => !value)}
            className="absolute right-2.5 top-1/2 -translate-y-1/2 rounded-lg p-1.5 text-neutral-400 hover:text-white hover:bg-white/5 transition"
          >
            {showPassword ? <EyeOff size={15} /> : <Eye size={15} />}
          </button>
        )}
      </div>
    </div>
  );
}
