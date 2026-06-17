import { motion } from "motion/react";

/**
 * ThinkingDots — three pulsing lime dots. Spring-based so they feel
 * softer than the old bounce keyframe.
 */
export function ThinkingDots() {
  return (
    <div
      className="flex items-center gap-1.5 py-1"
      aria-label="Assistant is thinking"
      role="status"
    >
      {[0, 1, 2].map((i) => (
        <motion.span
          key={i}
          className="h-2 w-2 rounded-full"
          style={{ background: "var(--color-lime)" }}
          animate={{
            opacity: [0.3, 1, 0.3],
            scale: [0.85, 1.15, 0.85],
          }}
          transition={{
            duration: 1.1,
            repeat: Infinity,
            delay: i * 0.16,
            ease: "easeInOut",
          }}
        />
      ))}
    </div>
  );
}
