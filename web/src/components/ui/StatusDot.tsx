import { cn } from "@/lib/utils";

type Status = "online" | "offline" | "checking" | "warning" | "active" | "idle";

const colors: Record<Status, string> = {
  online: "var(--color-success)",
  offline: "var(--color-coral)",
  checking: "var(--color-ink-faint)",
  warning: "var(--color-amber)",
  active: "var(--color-lime)",
  idle: "var(--color-ink-faint)",
};

const shouldPulse: Record<Status, boolean> = {
  online: true,
  offline: false,
  checking: true,
  warning: true,
  active: true,
  idle: false,
};

export function StatusDot({
  status,
  size = 8,
  className,
}: {
  status: Status;
  size?: number;
  className?: string;
}) {
  return (
    <span
      className={cn("relative inline-flex shrink-0", className)}
      style={{ width: size, height: size }}
      aria-label={status}
      role="status"
    >
      <span
        className="absolute inset-0 rounded-full"
        style={{
          background: colors[status],
          animation: shouldPulse[status]
            ? "pulse-dot 2s ease-in-out infinite"
            : undefined,
        }}
      />
      {shouldPulse[status] && (
        <span
          className="absolute inset-0 rounded-full animate-ping"
          style={{
            background: colors[status],
            opacity: 0.35,
            animationDuration: "2.4s",
          }}
        />
      )}
    </span>
  );
}
