import { motion } from "motion/react";
import { useNavigate } from "react-router-dom";
import { ArrowUpRight, Wrench, Thermometer, Repeat } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { StatusDot } from "@/components/ui/StatusDot";
import { type Role, roleAccents } from "@/components/ui/RoleBadge";
import { TopBar } from "@/components/layout/TopBar";
import { staggerContainer, staggerItem, springs } from "@/lib/motion";

interface AgentDetail {
  role: Role;
  name: string;
  tagline: string;
  description: string;
  longDescription: string;
  tools: string[];
  temperature: number;
  maxIterations: number;
  bestFor: string[];
  status: "active" | "idle";
}

const agents: AgentDetail[] = [
  {
    role: "generalist",
    name: "Sage",
    tagline: "Generalist · Default Fallback",
    description: "Research, explanation, routing, delegation.",
    longDescription:
      "Sage is the general-purpose agent and the system's default. Handles research, explanation, multi-domain questions, and anything that doesn't clearly fit a specialist. Knows when to delegate — hands code to Forge, UI to Lumen, tests to Sift, infra to Beacon, planning to Aria. Does not inherit Forge's no-prose rule, so it answers questions in plain language.",
    tools: ["terminal", "file", "http_client", "web_search", "calculator", "docs", "skill", "memory_recall", "remember", "delegation", "uploaded_file", "excel"],
    temperature: 0.4,
    maxIterations: 10,
    bestFor: ["Research", "Explanation", "Multi-domain tasks", "Delegation routing"],
    status: "active",
  },
  {
    role: "pm",
    name: "Aria",
    tagline: "Product Manager",
    description: "Plans, prioritizes, decomposes work.",
    longDescription:
      "Aria takes a fuzzy ask and turns it into a sharded plan the rest of the team can execute. Excels at scope negotiation, prioritization matrices, and writing crisp tickets with acceptance criteria.",
    tools: ["file", "http_client", "web_search", "uploaded_file", "excel"],
    temperature: 0.7,
    maxIterations: 6,
    bestFor: ["Decomposing epics", "Stakeholder comms", "Roadmap planning", "Spec writing"],
    status: "idle",
  },
  {
    role: "developer",
    name: "Forge",
    tagline: "Autonomous Developer",
    description: "Ships code. Verifies the fix.",
    longDescription:
      "Forge is your finish-the-ticket agent. Reads the file, runs the test, fixes the bug, verifies the fix, then stops. Strict about verification — never claims done without proof. Optimal temperature for precision coding.",
    tools: ["terminal", "file", "http_client", "web_search", "calculator", "security_scan", "uploaded_file", "excel"],
    temperature: 0.3,
    maxIterations: 15,
    bestFor: ["Bug fixes", "Feature implementation", "Refactoring", "Code review"],
    status: "active",
  },
  {
    role: "frontend",
    name: "Lumen",
    tagline: "Frontend Specialist",
    description: "Components, polish, design systems.",
    longDescription:
      "Lumen builds the UI. Reads the design tokens, follows existing component patterns, queries the docs before reinventing. Specialized in React, Tailwind, accessibility, and motion.",
    tools: ["terminal", "file", "browser", "docs", "web_search", "http_client", "memory_recall", "remember", "uploaded_file", "excel"],
    temperature: 0.3,
    maxIterations: 15,
    bestFor: ["Component builds", "Design system work", "Accessibility", "Animation"],
    status: "active",
  },
  {
    role: "qa",
    name: "Sift",
    tagline: "Quality Assurance",
    description: "Tests, scans, files precise repros.",
    longDescription:
      "Sift reads the diff, runs the tests, scans for OWASP issues, and ships the green check — or a precise repro if it can't. Conservative iteration budget keeps it focused on verification, not exploration.",
    tools: ["terminal", "file", "http_client", "calculator", "security_scan", "uploaded_file", "excel"],
    temperature: 0.6,
    maxIterations: 6,
    bestFor: ["Test suites", "Security scans", "Regression checks", "Bug repros"],
    status: "idle",
  },
  {
    role: "sre",
    name: "Beacon",
    tagline: "Site Reliability",
    description: "Incidents, deploys, monitoring.",
    longDescription:
      "Beacon owns the bridge between code and production. Diagnoses incidents from logs and metrics, ships the deploy, watches the dashboards. Generous iteration budget because incident response is rarely short.",
    tools: ["terminal", "file", "http_client", "web_search", "security_scan", "uploaded_file", "excel"],
    temperature: 0.6,
    maxIterations: 15,
    bestFor: ["Incident response", "Deploy pipelines", "Monitoring setup", "Log analysis"],
    status: "idle",
  },
];

export function AgentsPanel() {
  const navigate = useNavigate();
  return (
    <>
      <TopBar
        eyebrow="the cast"
        title="Agents"
        subtitle="Six roles — one generalist plus five specialists. Each tuned for a different shape of work."
      />
      <div className="px-6 lg:px-10 pb-16">
        <motion.div
          initial="hidden"
          animate="visible"
          variants={staggerContainer(0.1, 0.06)}
          className="grid grid-cols-1 lg:grid-cols-2 gap-5"
        >
          {agents.map((a) => (
            <AgentCard key={a.role} agent={a} onDelegate={() => navigate(`/chat?mode=agent&role=${a.role}`)} />
          ))}
        </motion.div>
      </div>
    </>
  );
}

function AgentCard({ agent, onDelegate }: { agent: AgentDetail; onDelegate: () => void }) {
  const accent = roleAccents[agent.role];
  return (
    <motion.div
      variants={staggerItem}
      transition={springs.gentle}
      whileHover={{ y: -4 }}
      className="glass-panel rounded-3xl p-6 relative overflow-hidden group"
      data-cursor="hover"
    >
      {/* Decorative gradient wash */}
      <div
        aria-hidden
        className="absolute -top-16 -right-16 h-48 w-48 rounded-full opacity-20 blur-3xl pointer-events-none transition-opacity duration-500 group-hover:opacity-40"
        style={{ background: accent.color }}
      />

      <div className="relative">
        {/* Header */}
        <div className="flex items-start gap-4 mb-5">
          <div
            className="h-14 w-14 rounded-2xl grid place-items-center font-display font-semibold text-xl shrink-0"
            style={{
              background: accent.bg,
              border: `1px solid ${accent.border}`,
              color: accent.color,
              fontFamily: "var(--font-display)",
              boxShadow: `0 8px 32px -10px ${accent.color}55`,
            }}
          >
            {agent.name[0]}
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-wrap">
              <h3
                className="text-2xl font-semibold leading-tight"
                style={{ fontFamily: "var(--font-display)" }}
              >
                {agent.name}
              </h3>
              <StatusDot status={agent.status} size={7} />
            </div>
            <p className="text-xs font-mono uppercase tracking-[0.15em]" style={{ color: accent.color }}>
              {agent.tagline}
            </p>
          </div>
        </div>

        {/* Description */}
        <p className="text-sm text-[var(--color-ink-muted)] leading-relaxed mb-5">
          {agent.longDescription}
        </p>

        {/* Meta stats */}
        <div className="grid grid-cols-2 gap-2 mb-5">
          <MetaTile
            icon={<Thermometer className="h-3 w-3" />}
            label="Temperature"
            value={agent.temperature.toFixed(1)}
          />
          <MetaTile
            icon={<Repeat className="h-3 w-3" />}
            label="Max iterations"
            value={agent.maxIterations.toString()}
          />
        </div>

        {/* Tools */}
        <div className="mb-5">
          <div className="flex items-center gap-1.5 mb-2 text-[10px] font-mono uppercase tracking-[0.15em] text-[var(--color-ink-faint)]">
            <Wrench className="h-3 w-3" />
            <span>{agent.tools.length} tools</span>
          </div>
          <div className="flex flex-wrap gap-1.5">
            {agent.tools.map((t) => (
              <span
                key={t}
                className="inline-flex items-center rounded-md border border-white/10 bg-white/5 px-2 py-0.5 text-[11px] font-mono text-[var(--color-ink-muted)]"
              >
                {t}
              </span>
            ))}
          </div>
        </div>

        {/* Best for */}
        <div className="mb-6">
          <div className="text-[10px] font-mono uppercase tracking-[0.15em] text-[var(--color-ink-faint)] mb-2">
            Best for
          </div>
          <div className="flex flex-wrap gap-1.5">
            {agent.bestFor.map((b) => (
              <Badge key={b} tone="outline">
                {b}
              </Badge>
            ))}
          </div>
        </div>

        {/* Delegate CTA */}
        <Button
          variant="primary"
          onClick={onDelegate}
          className="w-full group-hover:border-[var(--color-lime)]/30"
        >
          Delegate to {agent.name}
          <ArrowUpRight className="h-4 w-4" />
        </Button>
      </div>
    </motion.div>
  );
}

function MetaTile({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
}) {
  return (
    <div className="rounded-xl border border-white/10 bg-white/[0.03] p-3">
      <div className="flex items-center gap-1.5 text-[10px] font-mono uppercase tracking-wider text-[var(--color-ink-faint)] mb-1">
        {icon}
        {label}
      </div>
      <div className="text-lg font-semibold tabular" style={{ fontFamily: "var(--font-display)" }}>
        {value}
      </div>
    </div>
  );
}
