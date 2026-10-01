import { useState } from "react";
import { Eye, EyeOff } from "lucide-react";

export function OnuslyLogo() {
  return (
    <img
      src="https://see.fontimg.com/api/rf5/2nxo/MWIyNjZmNmYzMzQ5NGUyYThiMjRlNTZkNzg0MDE2YWUudHRm/T05VU0xZ/ghang.png?r=fs&h=131&w=1250&fg=FFFFFF&bg=292929&tb=1&s=105"
      alt="ONUSLY"
      className="
        w-[600px]
        h-auto
        object-contain
        drop-shadow-[0_8px_20px_rgba(0,0,0,0.35)]
      "
    />
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
    <div>
      <label className="mb-2 block text-sm font-semibold text-[#FFFFFF]">
        {label}
      </label>
      <div className="relative">
        <Icon
          size={20}
          className="absolute left-4 top-1/2 -translate-y-1/2 text-[#B5B5B5]"
        />
        <input
          type={isPassword && showPassword ? "text" : type}
          placeholder={placeholder}
          autoComplete={autoComplete}
          required
          className="w-full rounded-xl border border-[#777777] bg-[#4A4A4A] py-4 pl-12 pr-12 text-[15px] text-[#FFFFFF] placeholder:text-[#B5B5B5] outline-none shadow-[inset_0_1px_3px_rgba(0,0,0,0.25)] transition-all duration-200 focus:border-[#FFFFFF] focus:ring-2 focus:ring-white/10 hover:border-[#B5B5B5]"
        />
        {isPassword && (
          <button
            type="button"
            onClick={() => setShowPassword((value) => !value)}
            className="absolute right-3 top-1/2 -translate-y-1/2 rounded-lg p-2 text-[#B5B5B5] transition hover:bg-[#616161] hover:text-[#FFFFFF]"
          >
            {showPassword ? <EyeOff size={19} /> : <Eye size={19} />}
          </button>
        )}
      </div>
    </div>
  );
}
