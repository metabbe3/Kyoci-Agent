import { useRef, type ReactNode, type MouseEvent } from "react";
import { motion, useMotionValue, useSpring } from "motion/react";
import { springs } from "@/lib/motion";
import { cn } from "@/lib/utils";

/**
 * MagneticButton — pulls itself toward the cursor while the cursor is
 * inside its `radius` (px). Springs back to origin on exit. Wrap any
 * child (link, button, etc.) — pass `asChild` semantics via render-prop
 * is overkill; this wraps children in a motion.div.
 *
 * Great for hero CTAs where the pull feels magnetic and tactile.
 */
export function MagneticButton({
  children,
  className,
  strength = 0.4,
  radius = 80,
  onClick,
}: {
  children: ReactNode;
  className?: string;
  strength?: number;
  radius?: number;
  onClick?: (e: MouseEvent<HTMLDivElement>) => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const x = useMotionValue(0);
  const y = useMotionValue(0);
  const sx = useSpring(x, { stiffness: 250, damping: 18, mass: 0.4 });
  const sy = useSpring(y, { stiffness: 250, damping: 18, mass: 0.4 });

  const handleMove = (e: MouseEvent<HTMLDivElement>) => {
    const el = ref.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const cx = rect.left + rect.width / 2;
    const cy = rect.top + rect.height / 2;
    const dx = e.clientX - cx;
    const dy = e.clientY - cy;
    const dist = Math.hypot(dx, dy);
    // Falloff: full strength when cursor is on the button, fades by `radius`
    const intensity = Math.max(0, 1 - dist / radius);
    x.set(dx * strength * intensity);
    y.set(dy * strength * intensity);
  };

  const reset = () => {
    x.set(0);
    y.set(0);
  };

  return (
    <motion.div
      ref={ref}
      onMouseMove={handleMove}
      onMouseLeave={reset}
      onClick={onClick}
      style={{ x: sx, y: sy }}
      transition={springs.snappy}
      className={cn("inline-block", className)}
      data-cursor="hover"
    >
      {children}
    </motion.div>
  );
}
