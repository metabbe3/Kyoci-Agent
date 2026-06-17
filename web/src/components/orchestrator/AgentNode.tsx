import { motion } from "motion/react";
import { Link } from "react-router-dom";
import { ArrowUpRight } from "lucide-react";
import { type Role, roleAccents } from "@/components/ui/RoleBadge";
import { StatusDot } from "@/components/ui/StatusDot";
import { staggerItem, springs } from "@/lib/motion";
import { cn } from "@/lib/utils";

export interface AgentNodeProps {
  role: Role;
  name: string;
  tagline: string;
  description: string;
  toolCount: number;
  temperature: number;
  maxIterations: number;
  status?: "active" | "idle";
  delay?: number;
}

const icons: Record<Role, string> = {
  generalist: "GEN",
  pm: "PM",
  developer: "DEV",
  frontend: "UX",
  qa: "QA",
  sre: "SRE",
};

/**
 * AgentNode — one tile in the agent grid. Frosted glass card with a
 * gradient avatar in the role's accent color, status dot, role meta,
 * and a "Delegate" link to /chat?mode=agent&role=...
 */
export function AgentNode({
  role,
  name,
  tagline,
  description,
  toolCount,
  temperature,
  maxIterations,
  status = "idle",
  delay = 0,
}: AgentNodeProps) {
  const a = roleAccents[role];
  return (
    <motion.div variants={staggerItem} transition={{ ...springs.gentle, delay }}>
      <motion.div
        whileHover={{ y: -6, scale: 1.02 }}
        transition={springs.snappy}
        className="glass-panel rounded-2xl p-5 relative overflow-hidden group"
        data-cursor="hover"
      >
        {/* Accent glow */}
        <div
          aria-hidden
          className="absolute -top-12 -right-12 h-32 w-32 rounded-full opacity-30 blur-3xl pointer-events-none transition-opacity duration-500 group-hover:opacity-60"
          style={{ background: a.color }}
        />
        <div className="relative flex items-start gap-3 mb-3">
          {/* Avatar */}
          <div
            className="h-12 w-12 rounded-xl grid place-items-center font-display font-semibold text-base shrink-0"
            style={{
              background: a.bg,
              border: `1px solid ${a.border}`,
              color: a.color,
              fontFamily: "var(--font-display)",
            }}
          >
            {icons[role]}
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <h3
                className="text-lg font-semibold leading-tight"
                style={{ fontFamily: "var(--font-display)" }}
              >
                {name}
              </h3>
              <StatusDot status={status} size={6} />
            </div>
            <p className="text-xs text-[var(--color-ink-muted)]">{tagline}</p>
          </div>
        </div>
        <p className="text-[13px] leading-relaxed text-[var(--color-ink-muted)] mb-4">
          {description}
        </p>
        <div className="flex items-center gap-3 text-[10px] font-mono uppercase tracking-wider text-[var(--color-ink-faint)]">
          <span>{toolCount} tools</span>
          <span>·</span>
          <span>temp {temperature}</span>
          <span>·</span>
          <span>{maxIterations} iter</span>
        </div>
        <Link
          to={`/chat?mode=agent&role=${role}`}
          className="mt-4 inline-flex items-center gap-1 text-xs font-medium"
          style={{ color: a.color }}
          data-cursor="hover"
        >
          Delegate to {name}
          <ArrowUpRight className="h-3 w-3" />
        </Link>
      </motion.div>
    </motion.div>
  );
}
