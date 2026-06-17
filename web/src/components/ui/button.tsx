import { forwardRef, type ButtonHTMLAttributes } from "react";
import { motion } from "motion/react";
import { cn } from "@/lib/utils";
import { springs } from "@/lib/motion";

type Variant = "primary" | "secondary" | "outline" | "ghost" | "destructive" | "lime";
type Size = "default" | "sm" | "lg" | "icon" | "icon-sm";

const variants: Record<Variant, string> = {
  primary:
    "bg-white/10 text-[var(--color-ink)] border border-white/15 hover:bg-white/15 backdrop-blur-xl",
  lime:
    "bg-[var(--color-lime)] text-[var(--color-void)] border border-[var(--color-lime)] hover:brightness-110 shadow-[0_0_24px_-6px_rgba(198,244,50,0.6)]",
  secondary:
    "bg-white/5 text-[var(--color-ink-muted)] border border-white/10 hover:bg-white/10 hover:text-[var(--color-ink)] backdrop-blur-xl",
  outline:
    "bg-transparent text-[var(--color-ink)] border border-white/15 hover:bg-white/5",
  ghost:
    "bg-transparent text-[var(--color-ink-muted)] hover:bg-white/5 hover:text-[var(--color-ink)] border border-transparent",
  destructive:
    "bg-[var(--color-coral)] text-[var(--color-destructive-foreground)] border border-[var(--color-coral)] hover:brightness-110 shadow-[0_0_24px_-6px_rgba(255,107,90,0.5)]",
};

const sizes: Record<Size, string> = {
  default: "h-10 px-4 py-2 text-sm rounded-xl",
  sm: "h-8 px-3 text-xs rounded-lg",
  lg: "h-12 px-6 text-base rounded-xl",
  icon: "h-10 w-10 rounded-xl",
  "icon-sm": "h-8 w-8 rounded-lg",
};

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = "primary", size = "default", children, ...props }, ref) => (
    <motion.button
      ref={ref}
      whileHover={{ scale: 1.02 }}
      whileTap={{ scale: 0.97 }}
      transition={springs.snappy}
      data-cursor="hover"
      className={cn(
        "inline-flex items-center justify-center gap-2 whitespace-nowrap font-medium select-none transition-[background,border,color,filter] duration-200 ring-glow disabled:pointer-events-none disabled:opacity-40",
        variants[variant],
        sizes[size],
        className
      )}
      {...(props as any)}
    >
      {children}
    </motion.button>
  )
);
Button.displayName = "Button";
