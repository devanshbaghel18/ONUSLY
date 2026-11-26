import { useEffect, useRef, useState } from "react";
import { ArrowDown, ArrowUpRight } from "lucide-react";
import Brand from "./Brand";
import engaged from "../assets/film-engaged.webp";
import review from "../assets/film-review.webp";
import released from "../assets/film-released.webp";
import "./film.css";

const scenes = [
  { name: "Commitment", image: engaged, title: "Lock the", accent: "distractions.", copy: "Finish the album mix. Put Discord, Instagram and Steam under lock until your witness verifies the work." },
  { name: "Evidence", image: review, title: "Send the", accent: "proof.", copy: "Your final mix is with Maya. The recording is ready for review, and the latch stays shut." },
  { name: "Approval", image: released, title: "Release the", accent: "lock.", copy: "Maya approved the mix. The lock opens and the three apps bound to this commitment are released." },
];

export default function LockFilm({ onSignIn }) {
  const track = useRef(null);
  const stage = useRef(null);
  const [scene, setScene] = useState(0);

  useEffect(() => {
    let frame = null;
    function update() {
      frame = null;
      const travel = track.current.offsetHeight - stage.current.offsetHeight;
      const progress = Math.max(0, Math.min(2, -track.current.getBoundingClientRect().top / Math.max(1, travel) * 2));
      setScene(Math.round(progress));
      for (const [index, layer] of Array.from(stage.current.querySelectorAll(".film-layer")).entries()) {
        // Each incoming shot fades over the fully opaque previous exposure.
        layer.style.opacity = index === 0 ? 1 : Math.min(1, Math.max(0, (progress - index + .63) / .26));
        const offset = Math.min(1, Math.max(-1, progress - index));
        layer.style.setProperty("--push", 1.015 + (index === 2 ? -offset : offset + .5) * .09);
      }
    }
    function schedule() { if (frame === null) frame = requestAnimationFrame(update); }
    const observer = new ResizeObserver(schedule);
    observer.observe(track.current);
    window.addEventListener("scroll", schedule, { passive: true });
    update();
    return () => { observer.disconnect(); window.removeEventListener("scroll", schedule); cancelAnimationFrame(frame); };
  }, []);

  function select(index) {
    const start = scrollY + track.current.getBoundingClientRect().top;
    window.scrollTo({ top: start + index / 2 * (track.current.offsetHeight - stage.current.offsetHeight),
      behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth" });
  }

  const current = scenes[scene];
  return <section className="film-track" ref={track} aria-label="A sample ONUSLY commitment">
    <div className="film-stage" ref={stage}>
      <div className="film-screen" aria-hidden="true">{scenes.map((item,index) =>
        <div className="film-layer" key={item.name}><img src={item.image} fetchPriority={index === 0 ? "high" : "auto"} alt="" /></div>)}</div>
      <header className="film-header"><a href="#top" aria-label="ONUSLY home"><Brand /></a><nav><a href="#mechanism">Mechanism</a><a href="#enforcers">Enforcers</a><a href="#protocol">Protocol</a><a href="#catalog">Catalog</a><a href="#faq">FAQ</a><button onClick={onSignIn}>Sign in <ArrowUpRight size={16}/></button></nav></header>
      <div className="film-copy" key={scene}>
        <span className="instrument-label"><i/> Accountability app locker</span>
        <h1>{current.title}<br/><em>{current.accent}</em></h1>
        <p>{current.copy}</p>
        <button onClick={onSignIn} className="signal-button">Create a lock <ArrowUpRight size={18}/></button>
      </div>
      <footer className="film-timeline"><div className="film-guide"><span>Sample commitment</span><span>Scroll to explore <ArrowDown size={13}/></span></div>
        <nav aria-label="Commitment scenes">{scenes.map((item,i) => <button key={item.name} aria-pressed={i === scene} onClick={() => select(i)}><small>0{i+1}</small>{item.name}</button>)}</nav>
      </footer>
    </div>
  </section>;
}
