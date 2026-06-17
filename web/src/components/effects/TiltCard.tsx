import { useRef, type ReactNode } from "react";
import { motion, useMotionValue, useSpring, useTransform } from "motion/react";
import { springs } from "@/lib/motion";
import { cn } from "@/lib/utils";

/**
 * TiltCard — 3D pointer-driven tilt. Subtle (max ±8deg) so it never
 * feels gimmicky. Mouse position also drives a small lime sheen.
 *
 * On reduced-motion, the springs lock to 0 and it just renders a static
 * glass card.
 */
export function TiltCard({
  children,
  className,
  max = 8,
}: {
  children: ReactNode;
  className?: string;
  max?: number;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const rx = useMotionValue(0);
  const ry = useMotionValue(0);
  const mx = useMotionValue(50);
  const my = useMotionValue(50);

  const rotateX = useSpring(rx, { stiffness: 200, damping: 18 });
  const rotateY = useSpring(ry, { stiffness: 200, damping: 18 });

  const sheen = useTransform(
    [mx, my],
    ([x, y]) =>
      `radial-gradient(400px circle at ${x}% ${y}%, rgba(198, 244, 50, 0.18), transparent 55%)`
  );

  const handleMove = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const px = (e.clientX - rect.left) / rect.width; // 0..1
    const py = (e.clientY - rect.top) / rect.height;
    rx.set((0.5 - py) * (max * 2));
    ry.set((px - 0.5) * (max * 2));
    mx.set(px * 100);
    my.set(py * 100);
  };

  const handleLeave = () => {
    rx.set(0);
    ry.set(0);
    mx.set(50);
    my.set(50);
  };

  return (
    <motion.div
      ref={ref}
      onMouseMove={handleMove}
      onMouseLeave={handleLeave}
      style={{
        rotateX,
        rotateY,
        transformPerspective: 1000,
        transformStyle: "preserve-3d",
      }}
      transition={springs.gentle}
      className={cn(
        "glass-panel rounded-2xl relative overflow-hidden",
        className
      )}
      data-cursor="hover"
    >
      <motion.div
        aria-hidden
        className="pointer-events-none absolute inset-0 z-0"
        style={{ background: sheen }}
      />
      <div className="relative z-10" style={{ transform: "translateZ(40px)" }}>
        {children}
      </div>
    </motion.div>
  );
}
