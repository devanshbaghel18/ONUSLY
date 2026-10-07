import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import Login from "./auth/Login";
import SignUp from "./auth/SignUp";
import { OnuslyLogo } from "./auth/components";
import { isAuthenticated, setAuth } from "../lib/auth";
import { loginWithGoogle } from "../lib/api";

export default function Auth() {
  const navigate = useNavigate();
  const [isLogin, setIsLogin] = useState(true);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    // 1. Check if token returned via redirect query params (?token=...)
    const params = new URLSearchParams(window.location.search);
    const token = params.get("token");
    if (token) {
      setAuth(token, null);
      window.history.replaceState({}, document.title, window.location.pathname);
      navigate("/dashboard", { replace: true });
      return;
    }

    // 2. If already logged in, redirect to dashboard
    if (isAuthenticated()) {
      navigate("/dashboard", { replace: true });
    }
  }, [navigate]);

  const handleSubmit = (e) => {
    e.preventDefault();
    setNotice("Email sign-in isn't available yet. Please continue with Google.");
  };

  const handleGoogleSuccess = async (credentialResponse) => {
    try {
      if (!credentialResponse?.credential) {
        throw new Error("No credential received from Google.");
      }
      setNotice("Signing in with Google...");
      const data = await loginWithGoogle(credentialResponse.credential);
      setAuth(data.token, data.user);
      navigate("/dashboard", { replace: true });
    } catch (err) {
      console.error("Google sign-in failed:", err);
      setNotice(err.message || "Failed to sign in with Google");
    }
  };

  const handleGoogleError = () => {
    setNotice("Google Sign-In failed or was cancelled.");
  };

  const handleGoogleAuth = () => {
    const clientId = import.meta.env.VITE_GOOGLE_CLIENT_ID;
    const redirectUri = `${import.meta.env.VITE_API_BASE_URL}/auth/google/callback`;

    const params = new URLSearchParams({
      client_id: clientId,
      redirect_uri: redirectUri,
      response_type: "code",
      scope: "email profile",
      access_type: "offline",
    });

    window.location.href = `https://accounts.google.com/o/oauth2/v2/auth?${params.toString()}`;
  };

  return (
    <div className="relative min-h-screen overflow-hidden bg-[#292929]">
      {/* =====================================================
          SMOKY BACKGROUND
      ===================================================== */}
      <div className="pointer-events-none absolute -left-40 -top-40 h-[600px] w-[600px] rounded-full bg-[#616161]/20 blur-[120px]" />
      <div className="pointer-events-none absolute -right-40 top-[20%] h-[600px] w-[600px] rounded-full bg-[#777777]/10 blur-[130px]" />
      <div className="pointer-events-none absolute bottom-[-250px] left-[25%] h-[600px] w-[600px] rounded-full bg-[#3A3A3A] blur-[120px]" />

      {/* subtle texture */}
      <div
        className="pointer-events-none absolute inset-0 opacity-[0.045]"
        style={{
          backgroundImage: "radial-gradient(#ffffff 0.7px, transparent 0.7px)",
          backgroundSize: "5px 5px",
        }}
      />

      {/* =====================================================
          MAIN PAGE
      ===================================================== */}
      <main className="relative z-10 flex min-h-screen flex-col items-center px-4 pb-8 pt-8">
        {/* LOGO */}
        <div className="mb-5 flex justify-center">
          <OnuslyLogo />
        </div>

        {/* HEADER */}
        <div className="mb-7 text-center">
          <h1 className="text-3xl font-bold tracking-[-0.03em] text-[#FFFFFF]">
            {isLogin ? "Welcome Back" : "Create Your Account"}
          </h1>
          <p className="mt-2 text-[15px] text-[#B5B5B5]">
            {isLogin ? "Sign in to access your goals" : "Create an account and start your journey"}
          </p>
        </div>

        {/* AUTH CARD */}
        <div className="w-full max-w-[560px] rounded-[28px] border border-[#777777] bg-[#3A3A3A] p-7 sm:p-9 shadow-[0_25px_70px_rgba(0,0,0,0.45),inset_0_1px_0_rgba(255,255,255,0.06)]">
          {/* SIGN IN / SIGN UP SWITCHER */}
          <div className="mb-7 flex rounded-xl border border-[#777777] bg-[#292929] p-1">
            <button
              type="button"
              onClick={() => {
                setIsLogin(true);
                setNotice("");
              }}
              className={`flex-1 rounded-lg py-2.5 text-sm font-semibold transition-all ${
                isLogin ? "bg-[#616161] text-[#FFFFFF] shadow-md" : "text-[#B5B5B5] hover:text-[#FFFFFF]"
              }`}
            >
              Sign in
            </button>
            <button
              type="button"
              onClick={() => {
                setIsLogin(false);
                setNotice("");
              }}
              className={`flex-1 rounded-lg py-2.5 text-sm font-semibold transition-all ${
                !isLogin ? "bg-[#616161] text-[#FFFFFF] shadow-md" : "text-[#B5B5B5] hover:text-[#FFFFFF]"
              }`}
            >
              Sign up
            </button>
          </div>

          {/* DYNAMIC FORM */}
          {isLogin ? (
            <Login
              handleSubmit={handleSubmit}
              onGoogleSuccess={handleGoogleSuccess}
              onGoogleError={handleGoogleError}
              handleGoogleAuth={handleGoogleAuth}
              notice={notice}
            />
          ) : (
            <SignUp
              handleSubmit={handleSubmit}
              onGoogleSuccess={handleGoogleSuccess}
              onGoogleError={handleGoogleError}
              handleGoogleAuth={handleGoogleAuth}
              notice={notice}
            />
          )}
        </div>

        {/* Footer */}
        <p className="mt-8 text-center text-[10px] uppercase tracking-[0.25em] text-[#777777]">
          ONUSLY · FOCUS · CONSISTENCY · GROWTH
        </p>
      </main>
    </div>
  );
}