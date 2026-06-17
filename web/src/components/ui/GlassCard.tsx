import { forwardRef, type HTMLAttributes } from "react";
import { motion, type HTMLMotionProps } from "motion/react";
import { cn } from "@/lib/utils";
import { springs } from "@/lib/motion";

/**
 * GlassCard — the workhorse surface. A frosted panel that optionally
 * lifts on hover with a subtle lime under-glow. Used everywhere a static
 * Card would feel too inert.
 *
 * Pass `interactive` to enable the lift + glow. Pass `spotlight` to also
 * light up where the cursor is inside the card.
 */
export type GlassCardProps = Omit<HTMLMotionProps<"div">, "children"> & {
  interactive?: boolean;
  spotlight?: boolean;
  children?: React.ReactNode;
};

export const GlassCard = forwardRef<HTMLDivElement, GlassCardProps>(
  ({ className, interactive, spotlight, children, ...props }, ref) => {
    if (!interactive) {
      return (
        <div
          ref={ref as any}
          className={cn("glass-panel rounded-2xl", className)}
          {...(props as HTMLAttributes<HTMLDivElement>)}
        >
          {children}
        </div>
      );
    }
    return (
      <motion.div
        ref={ref}
        whileHover={{ y: -4, scale: 1.012 }}
        transition={springs.snappy}
        className={cn(
          "glass-panel rounded-2xl relative overflow-hidden",
          "hover:border-[var(--color-lime)]/30 hover:shadow-[0_24px_80px_-24px_rgba(198,244,50,0.25)]",
          "transition-[border,box-shadow] duration-300",
          className
        )}
        data-cursor="hover"
        {...props}
      >
        {spotlight && <SpotlightLayer />}
        <div className="relative z-10">{children}</div>
      </motion.div>
    );
  }
);
GlassCard.displayName = "GlassCard";

/**
 * SpotlightLayer — pure-CSS mouse-follow lime glow.
 * Updates a CSS var --mx/--my on mousemove; no React re-render.
 */
function SpotlightLayer() {
  const handleMove = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const el = (e.currentTarget as HTMLElement).parentElement;
    if (!el) return;
    el.style.setProperty("--mx", `${e.clientX - rect.left}px`);
    el.style.setProperty("--my", `${e.clientY - rect.top}px`);
  };
  return (
    <div
      aria-hidden
      onMouseMove={handleMove}
      className="pointer-events-auto absolute inset-0 z-0 opacity-0 transition-opacity duration-300 hover:opacity-100"
      style={{
        background:
          "radial-gradient(420px circle at var(--mx, 50%) var(--my, 50%), rgba(198, 244, 50, 0.12), transparent 60%)",
      }}
    />
  );
}
