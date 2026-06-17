import { type HTMLAttributes } from "react";
import { cn } from "@/lib/utils";

type Tone =
  | "default"
  | "lime"
  | "teal"
  | "coral"
  | "violet"
  | "success"
  | "warning"
  | "destructive"
  | "outline";

const tones: Record<Tone, string> = {
  default: "bg-white/8 text-[var(--color-ink-muted)] border-white/10",
  lime: "bg-[var(--color-lime)]/12 text-[var(--color-lime)] border-[var(--color-lime)]/25",
  teal: "bg-[var(--color-teal)]/12 text-[var(--color-teal)] border-[var(--color-teal)]/25",
  coral: "bg-[var(--color-coral)]/12 text-[var(--color-coral)] border-[var(--color-coral)]/25",
  violet:
    "bg-[var(--color-violet)]/12 text-[var(--color-violet)] border-[var(--color-violet)]/25",
  success:
    "bg-[var(--color-success)]/12 text-[var(--color-success)] border-[var(--color-success)]/25",
  warning:
    "bg-[var(--color-amber)]/12 text-[var(--color-amber)] border-[var(--color-amber)]/25",
  destructive:
    "bg-[var(--color-coral)]/15 text-[var(--color-coral)] border-[var(--color-coral)]/30",
  outline: "bg-transparent text-[var(--color-ink-muted)] border-white/15",
};

export function Badge({
  className,
  tone = "default",
  ...props
}: HTMLAttributes<HTMLSpanElement> & { tone?: Tone }) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-[11px] font-medium font-mono tracking-tight backdrop-blur-xl",
        tones[tone],
        className
      )}
      {...props}
    />
  );
}
