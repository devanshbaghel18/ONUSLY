import { Mail, Lock, ArrowRight } from "lucide-react";
import { Field } from "./components";

export default function Login({ handleSubmit, handleGoogleAuth, notice }) {
  return (
    <>
      <form onSubmit={handleSubmit} className="space-y-5">
        <Field
          label="EMAIL ADDRESS"
          icon={Mail}
          type="email"
          placeholder="Enter your Email Address"
          autoComplete="email"
        />

        <Field
          label="PASSWORD"
          icon={Lock}
          type="password"
          placeholder="Enter your password"
          autoComplete="current-password"
        />

        <div className="text-right">
          <a
            href="#"
            className="text-sm font-medium text-[#B5B5B5] transition hover:text-[#FFFFFF] hover:underline"
          >
            Forgot password?
          </a>
        </div>

        {notice && (
          <div className="rounded-xl border border-[#777777] bg-[#4A4A4A] px-4 py-3 text-sm text-[#B5B5B5]">
            {notice}
          </div>
        )}

        <button
          type="submit"
          className="group flex w-full items-center justify-center gap-3 rounded-xl bg-[#FFFFFF] py-4 text-[15px] font-bold text-[#292929] shadow-[0_8px_25px_rgba(0,0,0,0.3)] transition-all duration-200 hover:-translate-y-0.5 hover:bg-[#B5B5B5] active:translate-y-0"
        >
          SIGN IN
          <ArrowRight size={18} className="transition-transform group-hover:translate-x-1" />
        </button>
      </form>

      <div className="my-7 flex items-center gap-3">
        <div className="h-px flex-1 bg-[#777777]" />
        <span className="rounded-full border border-[#777777] bg-[#3A3A3A] px-3 py-1 text-sm text-[#B5B5B5]">
          Or continue with
        </span>
        <div className="h-px flex-1 bg-[#777777]" />
      </div>

      <button
        type="button"
        onClick={handleGoogleAuth}
        className="group flex w-full items-center justify-center gap-3 rounded-xl border border-[#777777] bg-[#4A4A4A] py-3.5 text-sm font-semibold text-[#FFFFFF] transition-all duration-200 hover:-translate-y-0.5 hover:border-[#B5B5B5] hover:bg-[#616161] active:translate-y-0"
      >
        <svg className="h-5 w-5" viewBox="0 0 24 24">
          <path fill="#FFFFFF" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" />
          <path fill="#FFFFFF" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
          <path fill="#FFFFFF" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" />
          <path fill="#FFFFFF" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" />
        </svg>
        GOOGLE
      </button>
    </>
  );
}
