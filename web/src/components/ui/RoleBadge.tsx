import { type HTMLAttributes } from "react";
import { cn } from "@/lib/utils";

export type Role = "pm" | "developer" | "frontend" | "qa" | "sre" | "generalist";

const accents: Record<
  Role,
  { color: string; bg: string; border: string; label: string; abbr: string }
> = {
  generalist: {
    color: "#a78bfa",
    bg: "rgba(167, 139, 250, 0.12)",
    border: "rgba(167, 139, 250, 0.28)",
    label: "Generalist",
    abbr: "GEN",
  },
  pm: {
    color: "#f472b6",
    bg: "rgba(244, 114, 182, 0.12)",
    border: "rgba(244, 114, 182, 0.28)",
    label: "Product Manager",
    abbr: "PM",
  },
  developer: {
    color: "#5eead4",
    bg: "rgba(94, 234, 212, 0.12)",
    border: "rgba(94, 234, 212, 0.28)",
    label: "Developer",
    abbr: "DEV",
  },
  frontend: {
    color: "#c6f432",
    bg: "rgba(198, 244, 50, 0.12)",
    border: "rgba(198, 244, 50, 0.28)",
    label: "Frontend",
    abbr: "UX",
  },
  qa: {
    color: "#ff6b5a",
    bg: "rgba(255, 107, 90, 0.12)",
    border: "rgba(255, 107, 90, 0.28)",
    label: "Quality Assurance",
    abbr: "QA",
  },
  sre: {
    color: "#fbbf24",
    bg: "rgba(251, 191, 36, 0.12)",
    border: "rgba(251, 191, 36, 0.28)",
    label: "Site Reliability",
    abbr: "SRE",
  },
};

export function RoleBadge({
  role,
  abbr = false,
  className,
  ...props
}: { role: Role; abbr?: boolean } & HTMLAttributes<HTMLSpanElement>) {
  const a = accents[role];
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-[11px] font-medium font-mono tracking-tight backdrop-blur-xl",
        className
      )}
      style={{ color: a.color, background: a.bg, borderColor: a.border }}
      {...props}
    >
      <span
        className="h-1.5 w-1.5 rounded-full"
        style={{ background: a.color }}
      />
      {abbr ? a.abbr : a.label}
    </span>
  );
}

export const roleAccents = accents;
