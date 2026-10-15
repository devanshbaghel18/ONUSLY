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
    <div className="relative min-h-screen overflow-hidden bg-[#050505] text-white selection:bg-white/20 selection:text-white flex flex-col justify-between">
      {/* =====================================================
          AMBIENT OBSIDIAN LIGHTING
      ===================================================== */}
      <div className="pointer-events-none fixed inset-0 z-0">
        <div className="absolute top-0 left-1/2 -translate-x-1/2 h-[500px] w-[800px] rounded-full bg-gradient-to-b from-white/[0.07] to-transparent blur-[120px]" />
        <div className="absolute bottom-0 right-1/4 h-[350px] w-[350px] rounded-full bg-white/[0.02] blur-[100px]" />
      </div>

      {/* Subtle Grid Pattern Overlay */}
      <div
        className="pointer-events-none absolute inset-0 z-0 opacity-[0.03]"
        style={{
          backgroundImage: "radial-gradient(#ffffff 1px, transparent 1px)",
          backgroundSize: "24px 24px",
        }}
      />

      {/* =====================================================
          MAIN CONTAINER
      ===================================================== */}
      <main className="relative z-10 flex min-h-screen flex-col items-center justify-center px-4 py-12">
        {/* BRAND LOGO */}
        <div className="mb-6 animate-fade-in">
          <OnuslyLogo />
        </div>

        {/* AUTH CARD */}
        <div className="w-full max-w-[480px] rounded-3xl border border-white/10 bg-[#0E0E12]/90 p-6 sm:p-8 backdrop-blur-3xl shadow-[0_30px_80px_-15px_rgba(0,0,0,0.9),inset_0_1px_0_rgba(255,255,255,0.1)] animate-scale-in">
          {/* TITLE & SUBTITLE */}
          <div className="mb-6 text-center">
            <h2 className="font-heading text-xl sm:text-2xl font-bold tracking-tight text-white">
              {isLogin ? "Welcome Back" : "Create Account"}
            </h2>
            <p className="mt-1 text-xs text-neutral-400">
              {isLogin
                ? "Enter your credentials to access your accountability dashboard"
                : "Set up your profile to start locking in on your targets"}
            </p>
          </div>

          {/* SIGN IN / SIGN UP PILL SWITCHER */}
          <div className="mb-6 flex rounded-xl border border-white/10 bg-[#08080A] p-1">
            <button
              type="button"
              onClick={() => {
                setIsLogin(true);
                setNotice("");
              }}
              className={`flex-1 rounded-lg py-2 text-xs font-bold transition-all duration-200 ${
                isLogin
                  ? "bg-white text-black shadow-md"
                  : "text-neutral-400 hover:text-white"
              }`}
            >
              Sign In
            </button>
            <button
              type="button"
              onClick={() => {
                setIsLogin(false);
                setNotice("");
              }}
              className={`flex-1 rounded-lg py-2 text-xs font-bold transition-all duration-200 ${
                !isLogin
                  ? "bg-white text-black shadow-md"
                  : "text-neutral-400 hover:text-white"
              }`}
            >
              Sign Up
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

          {/* SECURITY TRUST BADGE */}
          <div className="mt-6 pt-5 border-t border-white/[0.08] flex items-center justify-center gap-3 text-[10px] text-neutral-400 font-mono">
            <span className="flex items-center gap-1.5">
              <span className="h-1.5 w-1.5 rounded-full bg-white shadow-[0_0_6px_rgba(255,255,255,0.8)]" />
              <span>Encrypted Session</span>
            </span>
            <span>•</span>
            <span>Focus Lockout Engine</span>
          </div>
        </div>

        {/* FOOTER */}
        <p className="mt-8 text-center font-mono text-[10px] uppercase tracking-[0.25em] text-neutral-400">
          ONUSLY · HIGH-STAKES ACCOUNTABILITY ENGINE
        </p>
      </main>
    </div>
  );
}