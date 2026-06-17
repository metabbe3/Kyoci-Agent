/**
 * GrainOverlay — fine film grain via SVG feTurbulence.
 * Fixed, mix-blend-mode overlay, opacity 0.045. Adds the texture that
 * makes the glass panels feel like real material instead of flat CSS.
 *
 * The SVG is inlined as a data URI rather than rendered into the DOM so
 * it stays a single GPU-composited layer — important because the rest of
 * the app re-renders constantly (streaming chat, counters, etc.) and we
 * don't want grain repaints.
 */
const GRAIN_SVG = `data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='200' height='200'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='3' stitchTiles='stitch'/><feColorMatrix values='0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 0.55 0'/></filter><rect width='100%' height='100%' filter='url(%23n)'/></svg>`;

export function GrainOverlay() {
  return (
    <div
      aria-hidden
      className="pointer-events-none fixed inset-0 -z-10"
      style={{
        backgroundImage: `url("${GRAIN_SVG}")`,
        backgroundSize: "200px 200px",
        mixBlendMode: "overlay",
        opacity: 0.045,
      }}
    />
  );
}
