function App() {
  return (
    <main className="relative flex min-h-screen items-center justify-center overflow-hidden bg-[#f7f0e7] text-[#191512] antialiased">
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="floating-orb left-[8%] top-[14%] h-52 w-52 bg-[#d0b38f]/65" />
        <div className="floating-orb left-[20%] top-[68%] h-40 w-40 bg-[#9dc0b0]/50" style={{ animationDelay: '1.2s' }} />
        <div className="floating-orb right-[12%] top-[18%] h-64 w-64 bg-[#cabba5]/60" style={{ animationDelay: '2.4s' }} />
        <div className="floating-orb bottom-[10%] right-[25%] h-44 w-44 bg-[#d8af74]/45" style={{ animationDelay: '3s' }} />
      </div>

      <div className="hero-grid absolute inset-0 opacity-70" />

      <div className="relative z-10 text-center">
        <p className="mb-6 text-xs font-semibold uppercase tracking-[0.75em] text-[#635a53] animate-fade-up">
          Onusly
        </p>

        <h1 className="hero-title animate-fade-up text-[clamp(4.2rem,12vw,16rem)] font-black uppercase leading-[0.85] tracking-[-0.08em] text-[#17130f]">
          Welcome to
          <span className="hero-accent block">Onusly</span>
        </h1>

        <div className="mx-auto mt-8 h-1.5 w-32 rounded-full bg-gradient-to-r from-transparent via-[#d8af74] to-transparent opacity-80 animate-pulse-glow" />
      </div>
    </main>
  )
}

export default App