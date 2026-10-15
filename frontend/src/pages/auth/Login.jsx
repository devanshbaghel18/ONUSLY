import { Mail, Lock, ArrowRight } from "lucide-react";
import { GoogleLogin } from "@react-oauth/google";
import { Field } from "./components";

export default function Login({
  handleSubmit,
  onGoogleSuccess,
  onGoogleError,
  notice,
}) {
  return (
    <>
      <form onSubmit={handleSubmit} className="space-y-4">
        <Field
          label="Email Address"
          icon={Mail}
          type="email"
          placeholder="name@example.com"
          autoComplete="email"
        />

        <div className="space-y-1">
          <Field
            label="Password"
            icon={Lock}
            type="password"
            placeholder="••••••••••••"
            autoComplete="current-password"
          />
          <div className="flex justify-end pt-1">
            <a
              href="#"
              className="text-[11px] font-medium text-neutral-400 transition hover:text-white hover:underline"
            >
              Forgot password?
            </a>
          </div>
        </div>

        {notice && (
          <div className="rounded-xl border border-white/15 bg-white/[0.04] p-3 text-xs text-neutral-300 leading-relaxed">
            {notice}
          </div>
        )}

        <button
          type="submit"
          className="btn-glow group flex w-full items-center justify-center gap-2 rounded-xl bg-white py-3 text-xs font-bold uppercase tracking-wider text-black shadow-lg transition-all duration-200 hover:bg-neutral-200"
        >
          <span>Sign In</span>
          <ArrowRight size={14} className="transition-transform group-hover:translate-x-1" />
        </button>
      </form>

      <div className="my-5 flex items-center gap-3">
        <div className="h-px flex-1 bg-white/10" />
        <span className="rounded-full border border-white/10 bg-white/5 px-3 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-neutral-400">
          Or continue with
        </span>
        <div className="h-px flex-1 bg-white/10" />
      </div>

      <div className="flex w-full justify-center">
        <GoogleLogin
          onSuccess={onGoogleSuccess}
          onError={onGoogleError}
          theme="filled_black"
          size="large"
          shape="rectangular"
          text="signin_with"
          width="360"
        />
      </div>
    </>
  );
}
