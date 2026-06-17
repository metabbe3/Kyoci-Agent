import { forwardRef, type TextareaHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

export type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement>;

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, ...props }, ref) => (
    <textarea
      ref={ref}
      className={cn(
        "flex min-h-[60px] w-full rounded-xl border border-white/10 bg-white/5 px-3.5 py-2.5 text-sm text-[var(--color-ink)] backdrop-blur-xl",
        "placeholder:text-[var(--color-ink-faint)]",
        "transition-[border,box-shadow,background] duration-200",
        "hover:bg-white/[0.07] focus:bg-white/[0.07] focus:border-[var(--color-lime)]/40",
        "focus:outline-none focus:ring-2 focus:ring-[var(--color-lime)]/25",
        "disabled:cursor-not-allowed disabled:opacity-50",
        "resize-none",
        className
      )}
      {...props}
    />
  )
);
Textarea.displayName = "Textarea";
