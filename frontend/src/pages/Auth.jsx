import { useState, useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { GoogleLogin, useGoogleOAuth } from "@react-oauth/google";
import { LockKeyhole, FileCheck2, KeyRound, X } from "lucide-react";
import Brand from "../components/Brand";
import LockMechanism from "../components/LockMechanism";
import LockFilm from "../components/LockFilm";
import LandingEnforcers from "../components/landing/LandingEnforcers";
import LandingProtocol from "../components/landing/LandingProtocol";
import LandingCatalog from "../components/landing/LandingCatalog";
import LandingGuarantees from "../components/landing/LandingGuarantees";
import LandingFAQ from "../components/landing/LandingFAQ";
import LandingFooter from "../components/landing/LandingFooter";

import { isAuthenticated, setAuth } from "../lib/auth";
import { loginWithGoogle } from "../lib/api";

const phases = [
  { name: "Engage", icon: LockKeyhole, title: "Three apps locked.", body: "Discord, Instagram and Steam stay locked while you finish the mix.", evidence: "Awaiting your submission", state: "Lock engaged" },
  { name: "Submit", icon: FileCheck2, title: "The mix is with Maya.", body: "Your witness can listen to the recording. All three apps stay locked during review.", evidence: "final-mix.wav · Submitted", state: "Under review" },
  { name: "Release", icon: KeyRound, title: "Maya approved the mix.", body: "This commitment is complete. The latch lifts and its three restrictions are released.", evidence: "final-mix.wav · Approved", state: "Released" },
];

export default function Auth() {
  const navigate = useNavigate();
  const [phase, setPhase] = useState(0);
  const dialog = useRef(null);
  const { scriptLoadedSuccessfully } = useGoogleOAuth();
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

  function openSignIn() {
    setNotice("");
    dialog.current.showModal();
  }

  return (
    <main className="lock-entry" id="top">
      <LockFilm onSignIn={openSignIn} />
      <section className="explore" id="mechanism" aria-labelledby="explore-heading">
        <div className="explore-content">
          <span className="instrument-label"><i /> Try a sample commitment</span>
          <h2 id="explore-heading">Finish the<br /><em>album mix.</em></h2>
          <div className="phase-controls" aria-label="Preview stages">{phases.map(({name, icon: Icon}, i) => <button key={name} aria-pressed={i === phase} onClick={() => setPhase(i)}><Icon size={15} />{name}</button>)}</div>
          <div className="sample-decision" aria-live="polite"><h3>{phases[phase].title}</h3><p>{phases[phase].body}</p></div>
          <dl className="sample-receipt"><div><dt>Witness</dt><dd>@maya</dd></div><div><dt>Evidence</dt><dd>{phases[phase].evidence}</dd></div></dl>
        </div>
        <figure className="explore-model" data-phase={phase}>
          <div className="model-status"><i />{phases[phase].state}</div>
          <LockMechanism released={phase === 2} reviewing={phase === 1} demoApps interactive />
          <figcaption>Drag to rotate <span>← →</span></figcaption>
        </figure>
      </section>

      {/* Expanded Architectural Sections */}
      <LandingEnforcers onSignIn={openSignIn} />
      <LandingProtocol onSignIn={openSignIn} />
      <LandingCatalog onSignIn={openSignIn} />
      <LandingGuarantees />
      <LandingFAQ />
      <LandingFooter onSignIn={openSignIn} />

      <dialog ref={dialog} className="signin-dialog" aria-labelledby="signin-title" onClick={e => { if (e.target === dialog.current) dialog.current.close(); }}>
        <div className="signin-inner"><button className="dialog-close" aria-label="Close sign in" onClick={() => dialog.current.close()}><X /></button>
          <Brand /><h2 id="signin-title">Access your locks.</h2><p>Continue with Google to create or review a commitment.</p>
          <GoogleLogin onSuccess={handleGoogleSuccess} onError={handleGoogleError} text="continue_with" theme="filled_black" />
          <div role="status">{notice || (!scriptLoadedSuccessfully && "Waiting for Google sign-in. Check your connection if this persists.")}</div>
        </div>
      </dialog>
    </main>
  );
}
