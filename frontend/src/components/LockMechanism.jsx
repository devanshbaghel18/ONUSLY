import { useEffect, useRef } from "react";
import { LockKeyhole } from "lucide-react";
import { paintLock } from "./lockGeometry";

const initialView = { yaw: -.36, pitch: .54 };

export default function LockMechanism({ released = false, reviewing = false, interactive = false }) {
  const canvas = useRef(null);
  const view = useRef({ ...initialView, release: Number(released), review: Number(reviewing) });
  const redraw = useRef(null);
  const pointer = useRef(null);

  useEffect(() => {
    const element = canvas.current;
    const ctx = element.getContext("2d");
    if (!ctx) return;
    const motion = window.matchMedia("(prefers-reduced-motion: reduce)");
    let frame = null;

    function requestDraw() {
      if (frame === null) frame = requestAnimationFrame(render);
    }
    function render() {
      frame = null;
      const { clientWidth: width, clientHeight: height } = element;
      if (!width || !height) return;
      const ratio = Math.min(devicePixelRatio || 1, 2);
      const pixelWidth = Math.round(width * ratio);
      const pixelHeight = Math.round(height * ratio);
      if (element.width !== pixelWidth || element.height !== pixelHeight) {
        element.width = pixelWidth;
        element.height = pixelHeight;
      }
      ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
      const scene = view.current;
      let moving = false;
      for (const [key, target] of [["release", Number(released)], ["review", Number(reviewing)]]) {
        const delta = target - scene[key];
        if (motion.matches || Math.abs(delta) < .005) scene[key] = target;
        else {
          scene[key] += delta * .085;
          moving = true;
        }
      }
      paintLock(ctx, width, height, scene.yaw, scene.pitch, scene.release, scene.review);
      if (moving) requestDraw();
    }

    redraw.current = requestDraw;
    const resize = new ResizeObserver(requestDraw);
    resize.observe(element);
    requestDraw();
    return () => {
      resize.disconnect();
      if (frame !== null) cancelAnimationFrame(frame);
      redraw.current = null;
    };
  }, [released, reviewing]);

  function rotate(dx, dy) {
    view.current.yaw += dx;
    view.current.pitch = Math.min(1.2, Math.max(-1.2, view.current.pitch + dy));
    redraw.current?.();
  }

  function onKey(event) {
    const movement = { ArrowLeft: [-.14, 0], ArrowRight: [.14, 0], ArrowUp: [0, -.14], ArrowDown: [0,.14] };
    if (event.key === "Home") {
      event.preventDefault();
      Object.assign(view.current, initialView);
      redraw.current?.();
    } else if (movement[event.key]) {
      event.preventDefault();
      rotate(...movement[event.key]);
    }
  }

  return (
    <div className="mechanism-stage">
      <canvas ref={canvas} aria-hidden={!interactive} tabIndex={interactive ? 0 : undefined}
        aria-label={interactive ? "3D lock. Drag or use arrow keys to rotate. Home resets the view." : undefined}
        data-interactive={interactive || undefined}
        onKeyDown={interactive ? onKey : undefined}
        onPointerDown={interactive ? event => {
          if (!event.isPrimary || event.button !== 0) return;
          pointer.current = [event.clientX, event.clientY];
          event.currentTarget.setPointerCapture(event.pointerId);
        } : undefined}
        onPointerMove={interactive ? event => {
          if (!event.isPrimary || !pointer.current) return;
          const [x,y] = pointer.current;
          rotate((event.clientX - x) * .009, (event.clientY - y) * .009);
          pointer.current = [event.clientX, event.clientY];
        } : undefined}
        onLostPointerCapture={() => { pointer.current = null; }}>
        <LockKeyhole />
      </canvas>
    </div>
  );
}
