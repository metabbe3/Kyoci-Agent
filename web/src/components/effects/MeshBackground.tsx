/**
 * MeshBackground — two colored blobs drifting on a near-black canvas.
 * Fixed under everything (z -20), pointer-events none, never blocks UI.
 *
 * Trimmed from 4 blobs to 2 (lime + teal — the strongest brand hues) with
 * 50px blur and 60s drift loops. Each blob is its own GPU-composited layer
 * via will-change, so the drift animation never triggers layout on the
 * document above it. On reduced-motion the global guard nukes the keyframes.
 */
export function MeshBackground() {
  return (
    <div
      aria-hidden
      className="pointer-events-none fixed inset-0 -z-20 overflow-hidden"
      style={{ background: "var(--color-void)" }}
    >
      {/* Lime — top left */}
      <div
        className="absolute h-[40vw] w-[40vw] rounded-full"
        style={{
          top: "-15%",
          left: "-10%",
          background:
            "radial-gradient(circle at center, rgba(198,244,50,0.38) 0%, rgba(198,244,50,0) 65%)",
          filter: "blur(50px)",
          animation: "mesh-drift 60s ease-in-out infinite",
          willChange: "transform",
        }}
      />
      {/* Teal — bottom right */}
      <div
        className="absolute h-[40vw] w-[40vw] rounded-full"
        style={{
          bottom: "-15%",
          right: "-10%",
          background:
            "radial-gradient(circle at center, rgba(94,234,212,0.34) 0%, rgba(94,234,212,0) 65%)",
          filter: "blur(50px)",
          animation: "mesh-drift 60s ease-in-out infinite reverse",
          willChange: "transform",
        }}
      />
    </div>
  );
}
