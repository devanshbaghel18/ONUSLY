// Web Audio API procedural sound synthesizer for ONUSLY
// Zero external files, zero latency, runs entirely in-browser.

let audioCtx = null;

function getAudioContext() {
  if (typeof window === "undefined") return null;
  const AudioContext = window.AudioContext || window.webkitAudioContext;
  if (!AudioContext) return null;
  if (!audioCtx) {
    audioCtx = new AudioContext();
  }
  if (audioCtx.state === "suspended") {
    audioCtx.resume();
  }
  return audioCtx;
}

export function isSoundEnabled() {
  if (typeof window === "undefined") return true;
  const val = localStorage.getItem("onusly_sound_enabled");
  return val === null ? true : val === "true";
}

export function setSoundEnabled(enabled) {
  if (typeof window === "undefined") return;
  localStorage.setItem("onusly_sound_enabled", enabled ? "true" : "false");
}

export function toggleSound() {
  const current = isSoundEnabled();
  setSoundEnabled(!current);
  if (!current) {
    playLockClickSound();
  }
  return !current;
}

// 1. Heavy Mechanical Lock Engage (Resonant Thud + Dual Metallic Click)
export function playLockEngageSound() {
  if (!isSoundEnabled()) return;
  try {
    const ctx = getAudioContext();
    if (!ctx) return;
    const now = ctx.currentTime;

    // Sub-bass thud
    const osc1 = ctx.createOscillator();
    const gain1 = ctx.createGain();
    osc1.type = "sine";
    osc1.frequency.setValueAtTime(140, now);
    osc1.frequency.exponentialRampToValueAtTime(32, now + 0.18);
    gain1.gain.setValueAtTime(0.45, now);
    gain1.gain.exponentialRampToValueAtTime(0.001, now + 0.22);
    osc1.connect(gain1);
    gain1.connect(ctx.destination);
    osc1.start(now);
    osc1.stop(now + 0.22);

    // Mechanical metallic click
    const osc2 = ctx.createOscillator();
    const gain2 = ctx.createGain();
    osc2.type = "triangle";
    osc2.frequency.setValueAtTime(1100, now + 0.02);
    osc2.frequency.exponentialRampToValueAtTime(120, now + 0.09);
    gain2.gain.setValueAtTime(0.3, now + 0.02);
    gain2.gain.exponentialRampToValueAtTime(0.001, now + 0.1);
    osc2.connect(gain2);
    gain2.connect(ctx.destination);
    osc2.start(now + 0.02);
    osc2.stop(now + 0.1);
  } catch (err) {
    console.debug("[Audio] Lock engage sound skipped:", err);
  }
}

// 2. High-precision Relay Click (Proof Dispatch)
export function playProofSubmittedSound() {
  if (!isSoundEnabled()) return;
  try {
    const ctx = getAudioContext();
    if (!ctx) return;
    const now = ctx.currentTime;

    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.type = "sine";
    osc.frequency.setValueAtTime(880, now);
    osc.frequency.exponentialRampToValueAtTime(1760, now + 0.08);
    gain.gain.setValueAtTime(0.25, now);
    gain.gain.exponentialRampToValueAtTime(0.001, now + 0.12);
    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.start(now);
    osc.stop(now + 0.12);
  } catch (err) {
    console.debug("[Audio] Proof submitted sound skipped:", err);
  }
}

// 3. Solenoid Lock Release (Double-Action Clack + Chime)
export function playLockReleaseSound() {
  if (!isSoundEnabled()) return;
  try {
    const ctx = getAudioContext();
    if (!ctx) return;
    const now = ctx.currentTime;

    // Solenoid pop
    const osc1 = ctx.createOscillator();
    const gain1 = ctx.createGain();
    osc1.type = "sawtooth";
    osc1.frequency.setValueAtTime(450, now);
    osc1.frequency.exponentialRampToValueAtTime(70, now + 0.07);
    gain1.gain.setValueAtTime(0.35, now);
    gain1.gain.exponentialRampToValueAtTime(0.001, now + 0.09);
    osc1.connect(gain1);
    gain1.connect(ctx.destination);
    osc1.start(now);
    osc1.stop(now + 0.09);

    // Harmonic release chime (Major third)
    [523.25, 659.25].forEach((freq, i) => {
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.type = "sine";
      osc.frequency.setValueAtTime(freq, now + 0.06 + i * 0.03);
      gain.gain.setValueAtTime(0.18, now + 0.06 + i * 0.03);
      gain.gain.exponentialRampToValueAtTime(0.001, now + 0.35 + i * 0.03);
      osc.connect(gain);
      gain.connect(ctx.destination);
      osc.start(now + 0.06 + i * 0.03);
      osc.stop(now + 0.35 + i * 0.03);
    });
  } catch (err) {
    console.debug("[Audio] Lock release sound skipped:", err);
  }
}

// 4. Subtle Tactile Key Tick
export function playLockClickSound() {
  if (!isSoundEnabled()) return;
  try {
    const ctx = getAudioContext();
    if (!ctx) return;
    const now = ctx.currentTime;

    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.type = "triangle";
    osc.frequency.setValueAtTime(900, now);
    osc.frequency.exponentialRampToValueAtTime(300, now + 0.03);
    gain.gain.setValueAtTime(0.12, now);
    gain.gain.exponentialRampToValueAtTime(0.001, now + 0.035);
    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.start(now);
    osc.stop(now + 0.035);
  } catch (err) {
    console.debug("[Audio] Click sound skipped:", err);
  }
}
