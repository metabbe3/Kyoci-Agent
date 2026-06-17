import { forwardRef, type InputHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

export type InputProps = InputHTMLAttributes<HTMLInputElement>;

export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, ...props }, ref) => (
    <input
      ref={ref}
      className={cn(
        "flex h-10 w-full rounded-xl border border-white/10 bg-white/5 px-3.5 py-2 text-sm text-[var(--color-ink)] backdrop-blur-xl",
        "placeholder:text-[var(--color-ink-faint)]",
        "transition-[border,box-shadow,background] duration-200",
        "hover:bg-white/[0.07] focus:bg-white/[0.07] focus:border-[var(--color-lime)]/40",
        "focus:outline-none focus:ring-2 focus:ring-[var(--color-lime)]/25",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className
      )}
      {...props}
    />
  )
);
Input.displayName = "Input";
