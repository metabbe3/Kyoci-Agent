import type { Transition, Variants } from "motion/react";

/**
 * Spring presets — every interactive surface in Kyoci uses one of these.
 * Centralizing keeps the "feel" consistent: snappy for taps, gentle for
 * entrances, bouncy for delight moments.
 */
export const springs: Record<string, Transition> = {
  snappy: { type: "spring", stiffness: 500, damping: 30, mass: 0.8 },
  gentle: { type: "spring", stiffness: 120, damping: 18, mass: 0.6 },
  bouncy: { type: "spring", stiffness: 300, damping: 12, mass: 0.5 },
  lazy: { type: "spring", stiffness: 60, damping: 14, mass: 0.8 },
};

/** Page enter/exit — wraps every routed view. */
export const pageVariants: Variants = {
  initial: { opacity: 0, y: 14, filter: "blur(6px)" },
  animate: {
    opacity: 1,
    y: 0,
    filter: "blur(0px)",
    transition: { ...springs.gentle, duration: 0.4 },
  },
  exit: {
    opacity: 0,
    y: -8,
    filter: "blur(4px)",
    transition: { duration: 0.18 },
  },
};

/** Stagger children — wrap a list/grid container. */
export const staggerContainer = (delay = 0, each = 0.05): Variants => ({
  hidden: {},
  visible: {
    transition: { staggerChildren: each, delayChildren: delay },
  },
});

/** Single item inside a staggered container. */
export const staggerItem: Variants = {
  hidden: { opacity: 0, y: 18, filter: "blur(4px)" },
  visible: {
    opacity: 1,
    y: 0,
    filter: "blur(0px)",
    transition: springs.gentle,
  },
};

/** Generic fade-up for one-off reveals. */
export const fadeUp: Variants = {
  hidden: { opacity: 0, y: 12 },
  visible: { opacity: 1, y: 0, transition: springs.gentle },
};

/** Hover lift for cards. */
export const cardHover = {
  rest: { y: 0, scale: 1 },
  hover: { y: -4, scale: 1.015, transition: springs.snappy },
};

/** Slide-in for the sidebar active pill (shared element). */
export const layoutSlide = {
  transition: springs.snappy,
};
