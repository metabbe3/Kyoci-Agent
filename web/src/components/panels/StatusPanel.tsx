import { useEffect, useState } from "react";
import { motion } from "motion/react";
import { Activity, Server, Wrench, Sparkles, Brain } from "lucide-react";
import { TopBar } from "@/components/layout/TopBar";
import { Badge } from "@/components/ui/badge";
import { StatusDot } from "@/components/ui/StatusDot";
import { Counter } from "@/components/ui/Counter";
import { api } from "@/lib/api";
import { springs, staggerContainer, staggerItem } from "@/lib/motion";

export function StatusPanel() {
  const [status, setStatus] = useState<any>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const tick = () => {
      api.status()
        .then((s) => {
          if (!cancelled) {
            setStatus(s);
            setError(null);
          }
        })
        .catch((e) => !cancelled && setError(String(e)));
    };
    tick();
    const id = setInterval(tick, 5000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);

  const providers: string[] = status?.providers ?? [];
  const tools: any[] = status?.tools ?? [];
  const skills: any[] = status?.skills ?? [];

  return (
    <>
      <TopBar
        eyebrow="polling every 5s"
        title="Status"
        subtitle="Live view of the orchestrator — providers, tools, skills, memory."
      >
        {status?.status && (
          <Badge tone={status.started ? "lime" : "warning"}>
            <StatusDot status={status.started ? "online" : "warning"} size={6} />
            {status.status}
          </Badge>
        )}
      </TopBar>

      <div className="px-6 lg:px-10 pb-16 max-w-5xl">
        {error && (
          <div className="glass-panel rounded-2xl p-5 mb-6 border-[var(--color-coral)]/30 text-sm text-[var(--color-coral)]">
            Failed to load: {error}
          </div>
        )}

        {status && (
          <>
            <motion.div
              initial="hidden"
              animate="visible"
              variants={staggerContainer(0.05, 0.06)}
              className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-8"
            >
              <BigStat
                icon={<Server className="h-4 w-4" />}
                label="Providers"
                value={providers.length}
                delay={0}
              />
              <BigStat
                icon={<Wrench className="h-4 w-4" />}
                label="Tools"
                value={tools.length}
                delay={0.05}
              />
              <BigStat
                icon={<Sparkles className="h-4 w-4" />}
                label="Skills"
                value={skills.length}
                delay={0.1}
              />
              <BigStat
                icon={<Brain className="h-4 w-4" />}
                label="Memory entries"
                value={(status.memory_stats?.total_entries as number) ?? 0}
                delay={0.15}
              />
            </motion.div>

            {providers.length > 0 && (
              <motion.div
                initial={{ opacity: 0, y: 8 }}
                animate={{ opacity: 1, y: 0 }}
                transition={springs.gentle}
                className="glass-panel rounded-2xl p-5 mb-6"
              >
                <h3
                  className="text-base font-semibold mb-3"
                  style={{ fontFamily: "var(--font-display)" }}
                >
                  Registered providers
                </h3>
                <div className="flex flex-wrap gap-1.5">
                  {providers.map((p) => (
                    <motion.div
                      key={p}
                      initial={{ scale: 0.9, opacity: 0 }}
                      animate={{ scale: 1, opacity: 1 }}
                      transition={springs.snappy}
                    >
                      <Badge tone="lime" className="font-mono">
                        <StatusDot status="online" size={5} />
                        {p}
                      </Badge>
                    </motion.div>
                  ))}
                </div>
              </motion.div>
            )}

            {status.memory_stats && (
              <motion.div
                initial={{ opacity: 0, y: 8 }}
                animate={{ opacity: 1, y: 0 }}
                transition={springs.gentle}
                className="glass-panel rounded-2xl p-5"
              >
                <h3
                  className="text-base font-semibold mb-3"
                  style={{ fontFamily: "var(--font-display)" }}
                >
                  Memory
                </h3>
                <pre className="text-xs font-mono text-[var(--color-ink-muted)] bg-black/30 rounded-xl p-4 overflow-auto">
                  {JSON.stringify(status.memory_stats, null, 2)}
                </pre>
              </motion.div>
            )}

            <div className="mt-6 text-xs text-[var(--color-ink-faint)] font-mono flex items-center gap-2">
              <Activity className="h-3 w-3" />
              Last poll: {String(status.timestamp || "—")}
            </div>
          </>
        )}
      </div>
    </>
  );
}

function BigStat({
  icon,
  label,
  value,
  delay = 0,
}: {
  icon: React.ReactNode;
  label: string;
  value: number;
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
      <div className="flex items-center justify-between mb-3">
        <span className="text-[10px] font-mono uppercase tracking-[0.18em] text-[var(--color-ink-faint)]">
          {label}
        </span>
        <span className="h-7 w-7 grid place-items-center rounded-lg glass text-[var(--color-lime)]">
          {icon}
        </span>
      </div>
      <div
        className="text-4xl font-semibold tabular leading-none"
        style={{ fontFamily: "var(--font-display)" }}
      >
        <Counter value={value} />
      </div>
    </motion.div>
  );
}
