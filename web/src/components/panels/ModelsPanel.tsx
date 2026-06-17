import { useEffect, useMemo, useState } from "react";
import { motion } from "motion/react";
import { Search, Boxes, Wrench, Radio, Eye } from "lucide-react";
import { TopBar } from "@/components/layout/TopBar";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Counter } from "@/components/ui/Counter";
import { api } from "@/lib/api";
import type { ModelRow } from "@/lib/types";
import { springs, staggerContainer, staggerItem } from "@/lib/motion";
import { cn } from "@/lib/utils";

export function ModelsPanel() {
  const [models, setModels] = useState<ModelRow[]>([]);
  const [filter, setFilter] = useState("");
  const [providerFilter, setProviderFilter] = useState("all");

  useEffect(() => {
    api.models().then((r) => setModels(r.models)).catch(() => {});
  }, []);

  const providers = useMemo(
    () => Array.from(new Set(models.map((m) => m.provider))).sort(),
    [models]
  );

  const filtered = useMemo(() => {
    return models.filter((m) => {
      if (providerFilter !== "all" && m.provider !== providerFilter) return false;
      if (filter && !m.id.toLowerCase().includes(filter.toLowerCase())) return false;
      return true;
    });
  }, [models, filter, providerFilter]);

  const maxContext = useMemo(
    () => Math.max(1, ...models.map((m) => m.context_length || 0)),
    [models]
  );

  return (
    <>
      <TopBar
        eyebrow={`${models.length} models`}
        title="Models"
        subtitle="The full catalog across every live provider. Search, filter, compare context windows."
      />

      <div className="px-6 lg:px-10 pb-16">
        <motion.div
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={springs.gentle}
          className="flex flex-col sm:flex-row gap-3 mb-6"
        >
          <div className="relative flex-1 sm:max-w-md">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-[var(--color-ink-faint)]" />
            <Input
              placeholder="Filter by model id…"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              className="pl-9"
            />
          </div>
          <div className="flex gap-1 glass-panel rounded-xl p-1 overflow-x-auto">
            <FilterChip active={providerFilter === "all"} onClick={() => setProviderFilter("all")}>
              All ({models.length})
            </FilterChip>
            {providers.map((p) => (
              <FilterChip
                key={p}
                active={providerFilter === p}
                onClick={() => setProviderFilter(p)}
              >
                {p}
              </FilterChip>
            ))}
          </div>
        </motion.div>

        {filtered.length === 0 ? (
          <EmptyState />
        ) : (
          <motion.div
            initial="hidden"
            animate="visible"
            variants={staggerContainer(0.04, 0.04)}
            className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4"
          >
            {filtered.map((m, i) => (
              <motion.div
                key={`${m.provider}-${m.id}-${i}`}
                variants={staggerItem}
                transition={springs.gentle}
                whileHover={{ y: -4 }}
                data-cursor="hover"
                className="glass-panel rounded-2xl p-5"
              >
                <div className="flex items-start justify-between gap-2 mb-3">
                  <Badge tone="outline" className="font-mono">
                    {m.provider}
                  </Badge>
                  <div className="flex gap-1">
                    {m.supports_tools && <CapBadge icon={<Wrench className="h-3 w-3" />} label="tools" />}
                    {m.supports_streaming && <CapBadge icon={<Radio className="h-3 w-3" />} label="stream" />}
                    {m.supports_images && <CapBadge icon={<Eye className="h-3 w-3" />} label="vision" />}
                  </div>
                </div>
                <div
                  className="text-sm font-mono text-[var(--color-ink)] break-all leading-snug min-h-[2.5em]"
                >
                  {m.id}
                </div>
                {/* Context length bar */}
                <div className="mt-4">
                  <div className="flex items-baseline justify-between text-[10px] font-mono uppercase tracking-wider text-[var(--color-ink-faint)] mb-1.5">
                    <span>Context</span>
                    <span className="tabular text-[var(--color-ink-muted)]">
                      {m.context_length > 0 ? `${(m.context_length / 1000).toFixed(0)}k` : "—"}
                    </span>
                  </div>
                  <div className="h-1.5 rounded-full bg-white/5 overflow-hidden">
                    <motion.div
                      initial={{ width: 0 }}
                      animate={{
                        width: `${Math.min(100, ((m.context_length || 0) / maxContext) * 100)}%`,
                      }}
                      transition={{ ...springs.gentle, duration: 0.8 }}
                      className="h-full rounded-full"
                      style={{
                        background:
                          "linear-gradient(90deg, var(--color-lime), var(--color-teal))",
                      }}
                    />
                  </div>
                </div>
                <div className="mt-3 flex items-center justify-between text-[10px] font-mono uppercase tracking-wider text-[var(--color-ink-faint)]">
                  <span>Max output</span>
                  <span className="tabular text-[var(--color-ink-muted)]">
                    {m.max_output_tokens > 0 ? `${(m.max_output_tokens / 1000).toFixed(0)}k` : "—"}
                  </span>
                </div>
              </motion.div>
            ))}
          </motion.div>
        )}
      </div>
    </>
  );
}

function FilterChip({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      data-cursor="hover"
      className={cn(
        "inline-flex items-center px-3 py-1.5 text-xs rounded-lg whitespace-nowrap transition-all",
        active
          ? "bg-[var(--color-lime)] text-[var(--color-void)] font-medium"
          : "text-[var(--color-ink-muted)] hover:text-[var(--color-ink)]"
      )}
    >
      {children}
    </button>
  );
}

function CapBadge({ icon, label }: { icon: React.ReactNode; label: string }) {
  return (
    <span
      className="inline-flex items-center gap-1 rounded-md border border-[var(--color-teal)]/25 bg-[var(--color-teal)]/10 px-1.5 py-0.5 text-[10px] font-mono"
      style={{ color: "var(--color-teal)" }}
    >
      {icon}
      {label}
    </span>
  );
}

function EmptyState() {
  return (
    <motion.div
      initial={{ opacity: 0, scale: 0.95 }}
      animate={{ opacity: 1, scale: 1 }}
      transition={springs.gentle}
      className="glass-panel rounded-3xl p-12 text-center"
    >
      <div className="inline-grid place-items-center h-16 w-16 rounded-2xl glass mb-4">
        <Boxes className="h-7 w-7 text-[var(--color-ink-faint)]" />
      </div>
      <h3 className="text-lg font-semibold mb-1" style={{ fontFamily: "var(--font-display)" }}>
        No models yet
      </h3>
      <p className="text-sm text-[var(--color-ink-muted)] max-w-md mx-auto">
        Enable a provider in <Counter value={0} /> — wait, in <span className="text-[var(--color-lime)]">Providers</span> — and restart the server.
      </p>
    </motion.div>
  );
}
