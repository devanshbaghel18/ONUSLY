import { Mail, Lock, User, ArrowRight } from "lucide-react";
import { GoogleLogin } from "@react-oauth/google";
import { Field } from "./components";

export default function SignUp({
  handleSubmit,
  onGoogleSuccess,
  onGoogleError,
  notice,
}) {
  return (
    <>
      <form onSubmit={handleSubmit} className="space-y-5">
        <Field
          label="FULL NAME"
          icon={User}
          placeholder="Enter your Full Name"
          autoComplete="name"
        />

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
          autoComplete="new-password"
        />

        <Field
          label="CONFIRM PASSWORD"
          icon={Lock}
          type="password"
          placeholder="Confirm password"
          autoComplete="new-password"
        />

        {notice && (
          <div className="rounded-xl border border-[#777777] bg-[#4A4A4A] px-4 py-3 text-sm text-[#B5B5B5]">
            {notice}
          </div>
        )}

        <button
          type="submit"
          className="group flex w-full items-center justify-center gap-3 rounded-xl bg-[#FFFFFF] py-4 text-[15px] font-bold text-[#292929] shadow-[0_8px_25px_rgba(0,0,0,0.3)] transition-all duration-200 hover:-translate-y-0.5 hover:bg-[#B5B5B5] active:translate-y-0"
        >
          CREATE ACCOUNT
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

      <div className="flex w-full justify-center">
        <GoogleLogin
          onSuccess={onGoogleSuccess}
          onError={onGoogleError}
          theme="filled_black"
          size="large"
          shape="rectangular"
          text="signup_with"
          width="360"
        />
      </div>
    </>
  );
}
