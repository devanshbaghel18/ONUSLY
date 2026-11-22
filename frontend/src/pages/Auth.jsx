import { useState, useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { GoogleLogin, useGoogleOAuth } from "@react-oauth/google";
import { ArrowUpRight, ArrowDown, LockKeyhole, FileCheck2, KeyRound, X } from "lucide-react";
import Brand from "../components/Brand";
import LockMechanism from "../components/LockMechanism";
import lockPhoto from "../assets/lock-assembly.webp";

const phases = [
  { name: "Engage", icon: LockKeyhole, title: "Choose what stays locked.", body: "Commit to a task, select the apps and sites that distract you, and assign someone you trust to review your work." },
  { name: "Submit", icon: FileCheck2, title: "Put your work on record.", body: "Send your evidence to your witness. Your restrictions stay in place while they review it." },
  { name: "Release", icon: KeyRound, title: "Your witness turns the key.", body: "Approval releases this commitment’s restrictions. If your witness asks for changes, revise your work and submit again." },
];

import { isAuthenticated, setAuth } from "../lib/auth";
import { loginWithGoogle } from "../lib/api";

export default function Auth() {
  const navigate = useNavigate();
  const [phase, setPhase] = useState(0);
  const [inspect, setInspect] = useState(false);
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

  function tilt(event) {
    if (event.pointerType !== "mouse" || window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const box = event.currentTarget.getBoundingClientRect();
    event.currentTarget.style.setProperty("--yaw", ((event.clientX - box.left) / box.width - .5) * 11 + "deg");
    event.currentTarget.style.setProperty("--pitch", (.5 - (event.clientY - box.top) / box.height) * 8 + "deg");
  }

  return (
    <main className="lock-entry" id="top">
      <header className="entry-header"><a href="#top" aria-label="ONUSLY home"><Brand /></a>
        <nav><a href="#mechanism">How it works</a><button onClick={openSignIn}>Sign in <ArrowUpRight size={16} /></button></nav>
      </header>
      <section className="entry-hero">
        <div className="hero-copy">
          <span className="instrument-label"><i /> Accountability app locker</span>
          <h1>Focus under<br /><em>lock.</em></h1>
          <p>Lock your distractions behind a commitment. Get your apps back when someone you trust verifies the work.</p>
          <div className="entry-actions"><button className="signal-button" onClick={openSignIn}>Create a lock <ArrowUpRight size={19} /></button>
            <a href="#mechanism">Explore the lock <ArrowDown size={15} /></a></div>
        </div>
        <figure className="hero-figure" onPointerMove={tilt} onPointerLeave={e => { e.currentTarget.style.removeProperty("--yaw"); e.currentTarget.style.removeProperty("--pitch"); }}>
          {inspect ? <LockMechanism interactive /> : <img src={lockPhoto} alt="Three app icons held under glass by a steel latch and orange locking pin" fetchPriority="high" />}
          <figcaption><button aria-pressed={inspect} onClick={() => setInspect(value => !value)}>{inspect ? "Show photograph" : "Inspect in 3D"} <ArrowUpRight size={14} /></button></figcaption>
        </figure>
      </section>
      <section className="explore" id="mechanism" aria-labelledby="explore-heading">
        <div className="explore-content"><span className="instrument-label">Interactive preview</span><h2 id="explore-heading">Inside the lock</h2>
          <div className="phase-controls">{phases.map(({name, icon: Icon}, i) => <button key={name} aria-pressed={i === phase} onClick={() => setPhase(i)}><Icon size={15} />{name}</button>)}</div>
          <div aria-live="polite"><h3>{phases[phase].title}</h3><p>{phases[phase].body}</p></div>
        </div>
        <div className="explore-model"><LockMechanism released={phase === 2} reviewing={phase === 1} interactive /><p>Drag or use arrow keys to rotate</p></div>
      </section>
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
