import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { motion } from "motion/react";
import { ArrowUpRight, Sparkles, Plug, Boxes, Brain, Activity } from "lucide-react";
import { PipelineFlow } from "@/components/orchestrator/PipelineFlow";
import { AgentNode } from "@/components/orchestrator/AgentNode";
import { Counter } from "@/components/ui/Counter";
import { MagneticButton } from "@/components/effects/MagneticButton";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { api, health } from "@/lib/api";
import { staggerContainer, springs, staggerItem } from "@/lib/motion";

type BackendState = "checking" | "online" | "offline";

const agents = [
  {
    role: "generalist" as const,
    name: "Sage",
    tagline: "Generalist · Default",
    description: "Research, explanation, and routing. Handles anything that doesn't clearly fit a specialist — and knows when to delegate.",
    toolCount: 12,
    temperature: 0.4,
    maxIterations: 10,
  },
  {
    role: "pm" as const,
    name: "Aria",
    tagline: "Product Manager",
    description: "Plans, prioritizes, and turns fuzzy asks into sharded tickets the rest of the team can execute.",
    toolCount: 6,
    temperature: 0.7,
    maxIterations: 6,
  },
  {
    role: "developer" as const,
    name: "Forge",
    tagline: "Autonomous Developer",
    description: "Ships code. Reads the file, runs the test, fixes the bug, verifies the fix — all without hand-holding.",
    toolCount: 8,
    temperature: 0.3,
    maxIterations: 15,
  },
  {
    role: "frontend" as const,
    name: "Lumen",
    tagline: "Frontend Specialist",
    description: "Builds components, polishes UI, follows design-system patterns. Asks the docs before reinventing.",
    toolCount: 10,
    temperature: 0.3,
    maxIterations: 15,
  },
  {
    role: "qa" as const,
    name: "Sift",
    tagline: "Quality Assurance",
    description: "Reads the diff, runs the tests, scans for OWASP issues, files precise repros. Ships the green check.",
    toolCount: 7,
    temperature: 0.6,
    maxIterations: 6,
  },
  {
    role: "sre" as const,
    name: "Beacon",
    tagline: "Site Reliability",
    description: "Diagnoses incidents, ships deploys, watches metrics. Owns the bridge between code and production.",
    toolCount: 7,
    temperature: 0.6,
    maxIterations: 15,
  },
];

const activityFeed = [
  { who: "Sage", what: "Routed 'explain DNS' to research path, delegated infra fix to Beacon", when: "1m ago", tone: "violet" as const },
  { who: "Forge", what: "Refactored auth middleware into 3 modules", when: "2m ago", tone: "lime" as const },
  { who: "Lumen", what: "Built SettingsPanel with sticky save bar", when: "11m ago", tone: "teal" as const },
  { who: "Sift", what: "Flagged 2 OWASP issues in upload handler", when: "24m ago", tone: "coral" as const },
  { who: "Aria", what: "Decomposed 'redesign UI' into 9 sharded tasks", when: "1h ago", tone: "violet" as const },
];

export function OverviewPanel() {
  const navigate = useNavigate();
  const [providers, setProviders] = useState(0);
  const [models, setModels] = useState(0);
  const [backend, setBackend] = useState<BackendState>("checking");

  useEffect(() => {
    let cancelled = false;
    health()
      .then(() => !cancelled && setBackend("online"))
      .catch(() => !cancelled && setBackend("offline"));
    api.providers()
      .then((r) => {
        if (cancelled) return;
        setProviders(r.providers.filter((p) => p.available).length);
      })
      .catch(() => {});
    api.models()
      .then((r) => !cancelled && setModels(r.models.length))
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="px-6 lg:px-10 pb-16">
      {/* HERO ──────────────────────────────────────────────────────────── */}
      <section className="pt-14 lg:pt-20 pb-10">
        <motion.div
          initial="hidden"
          animate="visible"
          variants={staggerContainer(0, 0.07)}
        >
          {/* Eyebrow */}
          <motion.div variants={staggerItem} className="flex items-center gap-3 mb-6">
            <Badge tone={backend === "online" ? "lime" : "outline"}>
              <span
                className="h-1.5 w-1.5 rounded-full"
                style={{
                  background:
                    backend === "online" ? "var(--color-lime)" : "var(--color-ink-faint)",
                  animation: backend === "online" ? "pulse-dot 2s infinite" : undefined,
                }}
              />
              {backend === "online" ? "Orchestrator online" : backend === "offline" ? "Backend offline" : "Connecting…"}
            </Badge>
            <span className="text-[10px] font-mono uppercase tracking-[0.22em] text-[var(--color-ink-faint)]">
              v5 · liquid glass
            </span>
          </motion.div>

          {/* Wordmark */}
          <motion.h1
            variants={staggerItem}
            className="text-[18vw] sm:text-[14vw] lg:text-[11rem] xl:text-[13rem] leading-[0.82] font-bold tracking-tight"
            style={{ fontFamily: "var(--font-display)" }}
          >
            <span className="text-gradient-lime">KYOCI</span>
          </motion.h1>

          {/* Tagline + CTA row */}
          <motion.div
            variants={staggerItem}
            className="mt-6 flex flex-col lg:flex-row lg:items-end justify-between gap-6"
          >
            <p className="text-lg lg:text-xl text-[var(--color-ink-muted)] max-w-xl leading-snug">
              Five specialized agents. One orchestrator.
              <br />
              <span className="text-[var(--color-ink)]">Liquid smooth.</span>{" "}
              <span className="italic text-[var(--color-teal)]">Don't play safe.</span>
            </p>
            <div className="flex items-center gap-3 flex-wrap">
              <MagneticButton strength={0.35} radius={120}>
                <Button
                  variant="lime"
                  size="lg"
                  onClick={() => navigate("/chat?mode=agent")}
                  className="px-6"
                >
                  <Sparkles className="h-4 w-4" />
                  Start a task
                </Button>
              </MagneticButton>
              <Button
                variant="secondary"
                size="lg"
                onClick={() => navigate("/agents")}
              >
                Meet the agents
                <ArrowUpRight className="h-4 w-4" />
              </Button>
            </div>
          </motion.div>
        </motion.div>
      </section>

      {/* STATS ─────────────────────────────────────────────────────────── */}
      <motion.section
        initial="hidden"
        whileInView="visible"
        viewport={{ once: true, margin: "-80px" }}
        variants={staggerContainer(0.05, 0.05)}
        className="grid grid-cols-2 lg:grid-cols-4 gap-3 lg:gap-4 mb-14"
      >
        <StatCard icon={<Plug className="h-4 w-4" />} label="Providers live" delay={0}>
          <Counter value={providers} className="text-4xl lg:text-5xl" />
          <span className="text-2xl text-[var(--color-ink-faint)]"> / 20</span>
        </StatCard>
        <StatCard icon={<Boxes className="h-4 w-4" />} label="Models available" delay={0.05}>
          <Counter value={models} className="text-4xl lg:text-5xl" />
        </StatCard>
        <StatCard icon={<Activity className="h-4 w-4" />} label="Pipeline latency" delay={0.1}>
          <span className="text-4xl lg:text-5xl tabular">14</span>
          <span className="text-2xl text-[var(--color-ink-faint)]">ms</span>
        </StatCard>
        <StatCard icon={<Brain className="h-4 w-4" />} label="Skills loaded" delay={0.15}>
          <Counter value={20} className="text-4xl lg:text-5xl" />
        </StatCard>
      </motion.section>

      {/* PIPELINE ──────────────────────────────────────────────────────── */}
      <section className="mb-14">
        <SectionHeading
          eyebrow="orchestration"
          title="How a task flows"
          subtitle="Each LLM call gets one job. The orchestrator handles control flow so a 14B model can finish complex work reliably."
        />
        <motion.div
          initial={{ opacity: 0, y: 16 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={springs.gentle}
          className="glass-panel rounded-3xl p-6 lg:p-10"
        >
          <PipelineFlow active={backend === "online"} />
        </motion.div>
      </section>

      {/* AGENTS ────────────────────────────────────────────────────────── */}
      <section className="mb-14">
        <SectionHeading
          eyebrow="the cast"
          title="Five agents. Five specialties."
          subtitle="Each one is tuned for a different shape of work — temperature, tool access, and iteration budget all differ."
        />
        <motion.div
          initial="hidden"
          whileInView="visible"
          viewport={{ once: true, margin: "-80px" }}
          variants={staggerContainer(0.1, 0.07)}
          className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4"
        >
          {agents.map((a, i) => (
            <AgentNode key={a.role} {...a} delay={i * 0.04} />
          ))}
        </motion.div>
      </section>

      {/* ACTIVITY ──────────────────────────────────────────────────────── */}
      <section>
        <SectionHeading
          eyebrow="recent"
          title="What they shipped"
          subtitle="A rolling feed of the last few agent interactions."
        />
        <motion.div
          initial="hidden"
          whileInView="visible"
          viewport={{ once: true }}
          variants={staggerContainer(0.05, 0.06)}
          className="glass-panel rounded-2xl divide-y divide-white/5"
        >
          {activityFeed.map((row, i) => (
            <motion.div
              key={i}
              variants={staggerItem}
              className="flex items-center gap-4 px-5 py-3.5"
            >
              <Badge tone={row.tone}>{row.who}</Badge>
              <span className="flex-1 text-sm text-[var(--color-ink)] truncate">
                {row.what}
              </span>
              <span className="text-[10px] font-mono uppercase tracking-wider text-[var(--color-ink-faint)] shrink-0">
                {row.when}
              </span>
            </motion.div>
          ))}
        </motion.div>
      </section>
    </div>
  );
}

function SectionHeading({
  eyebrow,
  title,
  subtitle,
}: {
  eyebrow: string;
  title: string;
  subtitle?: string;
}) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true }}
      transition={springs.gentle}
      className="mb-5 flex flex-col gap-2"
    >
      <span className="text-[10px] font-mono uppercase tracking-[0.22em] text-[var(--color-lime)]">
        {eyebrow}
      </span>
      <h2
        className="text-3xl lg:text-4xl font-semibold tracking-tight"
        style={{ fontFamily: "var(--font-display)" }}
      >
        {title}
      </h2>
      {subtitle && (
        <p className="text-sm text-[var(--color-ink-muted)] max-w-2xl">{subtitle}</p>
      )}
    </motion.div>
  );
}

function StatCard({
  icon,
  label,
  children,
  delay = 0,
}: {
  icon: React.ReactNode;
  label: string;
  children: React.ReactNode;
  delay?: number;
}) {
  return (
    <motion.div
      variants={staggerItem}
      transition={{ ...springs.gentle, delay }}
      whileHover={{ y: -4 }}
      className="glass-panel rounded-2xl p-5"
      data-cursor="hover"
    >
      <div className="flex items-center gap-2 mb-3 text-[var(--color-ink-muted)]">
        <span className="h-7 w-7 grid place-items-center rounded-lg glass">
          {icon}
        </span>
        <span className="text-[10px] uppercase tracking-[0.18em] font-mono">
          {label}
        </span>
      </div>
      <div
        className="flex items-baseline gap-1 font-bold tracking-tight"
        style={{ fontFamily: "var(--font-display)" }}
      >
        {children}
      </div>
    </motion.div>
  );
}
