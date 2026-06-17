import { motion } from "motion/react";
import { springs } from "@/lib/motion";

/**
 * PipelineFlow — animated SVG of the orchestrator pipeline.
 * Planner → Dispatcher → Workers (×3) → Synthesizer. Connectors are
 * dashed lines that flow via stroke-dashoffset animation. When `active`,
 * the workers pulse and the whole pipeline gets a lime under-glow.
 *
 * Pure decoration, fully aria-hidden. Fixed viewBox so it scales to its
 * container width while keeping the layout coherent.
 */
export function PipelineFlow({ active = false }: { active?: boolean }) {
  const stages = [
    { label: "Planner", subLabel: "decompose", x: 60 },
    { label: "Dispatcher", subLabel: "route", x: 240 },
    { label: "Workers", subLabel: "parallel", x: 420 },
    { label: "Synthesizer", subLabel: "merge", x: 600 },
  ];

  return (
    <div className="relative w-full" aria-hidden>
      <svg
        viewBox="0 0 680 180"
        className="w-full h-auto"
        style={{ maxHeight: 220 }}
      >
        <defs>
          <linearGradient id="flow-line" x1="0" x2="1" y1="0" y2="0">
            <stop offset="0%" stopColor="rgba(198, 244, 50, 0.0)" />
            <stop offset="20%" stopColor="rgba(198, 244, 50, 0.5)" />
            <stop offset="80%" stopColor="rgba(94, 234, 212, 0.5)" />
            <stop offset="100%" stopColor="rgba(94, 234, 212, 0.0)" />
          </linearGradient>
          <filter id="glow">
            <feGaussianBlur stdDeviation="3" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>

        {/* Connector lines (flowing dashes) */}
        {stages.slice(0, -1).map((s, i) => {
          const next = stages[i + 1];
          return (
            <line
              key={i}
              x1={s.x + 40}
              y1={90}
              x2={next.x - 40}
              y2={90}
              stroke="url(#flow-line)"
              strokeWidth={2}
              strokeDasharray="6 6"
              style={{
                animation: "flow-dash 1.4s linear infinite",
                opacity: active ? 1 : 0.55,
              }}
            />
          );
        })}

        {/* Stage nodes */}
        {stages.map((s, i) => {
          const isWorkers = s.label === "Workers";
          return (
            <motion.g
              key={s.label}
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ ...springs.gentle, delay: 0.1 + i * 0.08 }}
            >
              {/* Workers gets three stacked circles */}
              {isWorkers ? (
                <>
                  {[0, 1, 2].map((j) => (
                    <motion.circle
                      key={j}
                      cx={s.x}
                      cy={90 + (j - 1) * 28}
                      r={11}
                      fill="rgba(198, 244, 50, 0.12)"
                      stroke="var(--color-lime)"
                      strokeWidth={1.5}
                      animate={
                        active
                          ? { scale: [1, 1.15, 1], opacity: [0.6, 1, 0.6] }
                          : { opacity: 0.6 }
                      }
                      transition={{
                        duration: 1.5,
                        repeat: Infinity,
                        delay: j * 0.18,
                      }}
                    />
                  ))}
                </>
              ) : (
                <motion.circle
                  cx={s.x}
                  cy={90}
                  r={22}
                  fill="rgba(255, 255, 255, 0.04)"
                  stroke={active ? "var(--color-lime)" : "rgba(255,255,255,0.2)"}
                  strokeWidth={1.5}
                  filter={active ? "url(#glow)" : undefined}
                  animate={
                    active
                      ? {
                          boxShadow: "0 0 24px rgba(198,244,50,0.5)",
                        }
                      : {}
                  }
                />
              )}
              {/* Label */}
              <text
                x={s.x}
                y={isWorkers ? 150 : 138}
                textAnchor="middle"
                className="fill-[var(--color-ink)]"
                style={{
                  fontFamily: "var(--font-display)",
                  fontSize: 13,
                  fontWeight: 600,
                }}
              >
                {s.label}
              </text>
              <text
                x={s.x}
                y={isWorkers ? 166 : 154}
                textAnchor="middle"
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 9,
                  fill: "var(--color-ink-faint)",
                  textTransform: "uppercase",
                  letterSpacing: "0.1em",
                }}
              >
                {s.subLabel}
              </text>
            </motion.g>
          );
        })}
      </svg>
    </div>
  );
}
