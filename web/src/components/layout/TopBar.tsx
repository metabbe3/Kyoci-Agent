import { motion } from "motion/react";
import { springs } from "@/lib/motion";

/**
 * TopBar — sticky page header. Renders the page title in Clash Display,
 * an optional subtitle/tagline, and a slot for contextual actions on
 * the right. Animates in on every route change (page-level enter).
 */
export function TopBar({
  eyebrow,
  title,
  subtitle,
  children,
}: {
  eyebrow?: string;
  title: string;
  subtitle?: string;
  children?: React.ReactNode;
}) {
  return (
    <motion.header
      initial={{ opacity: 0, y: -8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={springs.gentle}
      className="sticky top-0 z-30 px-6 lg:px-10 pt-10 pb-6"
    >
      <div
        className="glass-panel rounded-2xl px-5 py-4 flex items-center gap-4 flex-wrap"
      >
        <div className="flex flex-col min-w-0 flex-1">
          {eyebrow && (
            <span className="text-[10px] uppercase tracking-[0.22em] text-[var(--color-lime)] font-mono">
              {eyebrow}
            </span>
          )}
          <h1
            className="text-[1.75rem] lg:text-[2rem] leading-none font-semibold tracking-tight"
            style={{ fontFamily: "var(--font-display)" }}
          >
            {title}
          </h1>
          {subtitle && (
            <p className="text-sm text-[var(--color-ink-muted)] mt-1">
              {subtitle}
            </p>
          )}
        </div>
        {children && (
          <div className="flex items-center gap-2 flex-wrap">{children}</div>
        )}
      </div>
    </motion.header>
  );
}
