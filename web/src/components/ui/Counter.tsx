import { useEffect, useRef } from "react";
import { animate, useInView, useMotionValue, useTransform, motion } from "motion/react";

/**
 * Counter — animates from 0 to `value` when scrolled into view.
 * Renders tabular numerals in the display font so big stats look the part.
 *
 * `format` lets callers add suffixes (e.g. "k", "%") or thousands separators.
 */
export function Counter({
  value,
  duration = 1.2,
  format = (n: number) => Math.round(n).toString(),
  className,
}: {
  value: number;
  duration?: number;
  format?: (n: number) => string;
  className?: string;
}) {
  const ref = useRef<HTMLSpanElement>(null);
  const inView = useInView(ref, { once: true, margin: "-50px" });
  const mv = useMotionValue(0);
  const text = useTransform(mv, (latest) => format(latest));

  useEffect(() => {
    if (!inView) return;
    const controls = animate(mv, value, {
      duration,
      ease: [0.16, 1, 0.3, 1],
    });
    return () => controls.stop();
  }, [inView, value, duration, mv]);

  return (
    <motion.span
      ref={ref}
      className={className}
      style={{ fontVariantNumeric: "tabular-nums" }}
    >
      {text}
    </motion.span>
  );
}
