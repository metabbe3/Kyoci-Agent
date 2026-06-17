import { useEffect, useMemo, useState } from "react";
import { motion } from "motion/react";
import { Plug, Zap, RefreshCw, CheckCircle2, XCircle, Cloud, Server, Network } from "lucide-react";
import { TopBar } from "@/components/layout/TopBar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { api } from "@/lib/api";
import type { ProviderSummary, ProviderConfigDTO } from "@/lib/types";
import { springs, staggerContainer, staggerItem } from "@/lib/motion";
import { cn } from "@/lib/utils";
import { toast } from "sonner";

type Filter = "all" | "cloud" | "local" | "gateway";

const providerType: Record<string, Filter> = {
  ollama: "local",
  lmstudio: "local",
  litellm: "gateway",
  openrouter: "gateway",
};

const providerColor: Record<string, string> = {
  openai: "#10a37f",
  anthropic: "#d97757",
  ollama: "#fed7aa",
  lmstudio: "#a78bfa",
  gemini: "#4285f4",
  zai: "#38bdf8",
  groq: "#f55036",
  mistral: "#ff7000",
  deepseek: "#4d6bfe",
  together: "#0f6fff",
  fireworks: "#ef4444",
  xai: "#ffffff",
  cohere: "#39594d",
  perplexity: "#20808d",
  openrouter: "#6366f1",
  litellm: "#a3e635",
  cloudflare: "#f38020",
  nim: "#76b900",
  moonshot: "#e2e2e2",
  qwen: "#615ced",
};

function typeOf(name: string): Filter {
  return providerType[name] ?? "cloud";
}

export function ProvidersPanel() {
  const [providers, setProviders] = useState<ProviderSummary[]>([]);
  const [config, setConfig] = useState<Record<string, ProviderConfigDTO>>({});
  const [filter, setFilter] = useState<Filter>("all");
  const [search, setSearch] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);
  const [testing, setTesting] = useState<string | null>(null);
  const [dirty, setDirty] = useState<Record<string, ProviderConfigDTO>>({});

  const refresh = () => {
    Promise.all([api.providers(), api.getConfig()])
      .then(([p, c]) => {
        setProviders(p.providers);
        setConfig(c.providers);
      })
      .catch(() => {});
  };
  useEffect(refresh, []);

  const filtered = useMemo(() => {
    return providers
      .filter((p) => filter === "all" || typeOf(p.name) === filter)
      .filter((p) => !search || p.name.includes(search.toLowerCase()))
      .sort((a, b) => Number(b.available) - Number(a.available));
  }, [providers, filter, search]);

  const stats = useMemo(() => {
    const enabled = providers.filter((p) => p.available).length;
    const total = providers.length;
    return { enabled, total, disabled: total - enabled };
  }, [providers]);

  const setField = (name: string, patch: Partial<ProviderConfigDTO>) => {
    const base = dirty[name] ?? config[name];
    if (!base) return;
    setDirty((d) => ({ ...d, [name]: { ...base, ...patch } }));
  };

  const save = async (name: string) => {
    const dto = dirty[name];
    if (!dto) return;
    try {
      await api.putConfig({ [name]: dto });
      toast.success(`${name} saved`, {
        description: "Restart the server for changes to take effect.",
      });
      setDirty((d) => {
        const next = { ...d };
        delete next[name];
        return next;
      });
      refresh();
    } catch (e: any) {
      toast.error("Save failed", { description: e.message });
    }
  };

  const test = async (name: string) => {
    setTesting(name);
    try {
      const r = await api.testConnection(name);
      if (r.available) {
        toast.success(`${name} reachable`, {
          description: "Provider responded to auth + headers check.",
        });
      } else {
        toast.error(`${name} not reachable`, { description: r.error });
      }
    } catch (e: any) {
      toast.error("Test failed", { description: e.message });
    } finally {
      setTesting(null);
    }
  };

  return (
    <>
      <TopBar
        eyebrow={`${stats.enabled} / ${stats.total} live`}
        title="Providers"
        subtitle="Twenty LLM gateways. Cloud, local, and aggregator — all OpenAI-compatible."
      >
        <Button variant="secondary" size="sm" onClick={refresh} data-cursor="hover">
          <RefreshCw className="h-3.5 w-3.5" />
          Refresh
        </Button>
      </TopBar>

      <div className="px-6 lg:px-10 pb-16">
        {/* Filters + search */}
        <motion.div
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={springs.gentle}
          className="flex flex-col sm:flex-row gap-3 mb-6"
        >
          <div className="flex gap-1 glass-panel rounded-xl p-1">
            {([
              ["all", "All", null],
              ["cloud", "Cloud", <Cloud className="h-3 w-3" key="c" />],
              ["local", "Local", <Server className="h-3 w-3" key="l" />],
              ["gateway", "Gateway", <Network className="h-3 w-3" key="g" />],
            ] as const).map(([key, label, icon]) => (
              <button
                key={key}
                onClick={() => setFilter(key as Filter)}
                data-cursor="hover"
                className={cn(
                  "inline-flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg transition-all",
                  filter === key
                    ? "bg-[var(--color-lime)] text-[var(--color-void)] font-medium"
                    : "text-[var(--color-ink-muted)] hover:text-[var(--color-ink)]"
                )}
              >
                {icon}
                {label}
              </button>
            ))}
          </div>
          <Input
            placeholder="Search providers…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="sm:w-64"
          />
        </motion.div>

        {/* Provider grid */}
        <motion.div
          initial="hidden"
          animate="visible"
          variants={staggerContainer(0.05, 0.04)}
          className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4"
        >
          {filtered.map((p) => {
            const isDirty = !!dirty[p.name];
            const working = dirty[p.name] ?? config[p.name];
            const color = providerColor[p.name] ?? "#9ba1ad";
            if (!working) return null;
            return (
              <motion.div
                key={p.name}
                variants={staggerItem}
                transition={springs.gentle}
                whileHover={{ y: -4 }}
                className="glass-panel rounded-2xl overflow-hidden"
                data-cursor="hover"
              >
                <div
                  onClick={() => setExpanded(expanded === p.name ? null : p.name)}
                  className="cursor-pointer p-5 flex items-center gap-3"
                >
                  <div
                    className="h-11 w-11 rounded-xl grid place-items-center font-display font-semibold uppercase text-sm shrink-0"
                    style={{
                      background: `${color}22`,
                      border: `1px solid ${color}55`,
                      color: color,
                    }}
                  >
                    {p.name.slice(0, 2)}
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <h3
                        className="text-base font-semibold"
                        style={{ fontFamily: "var(--font-display)" }}
                      >
                        {p.name}
                      </h3>
                      {p.available ? (
                        <Badge tone="lime">
                          <CheckCircle2 className="h-3 w-3" />
                          live
                        </Badge>
                      ) : working.enabled ? (
                        <Badge tone="warning">enabled · unverified</Badge>
                      ) : (
                        <Badge tone="outline">
                          <XCircle className="h-3 w-3" />
                          off
                        </Badge>
                      )}
                    </div>
                    <div className="flex items-center gap-2 mt-0.5 text-[11px] font-mono text-[var(--color-ink-faint)] truncate">
                      <TagIcon type={typeOf(p.name)} />
                      <span className="truncate">{p.base_url}</span>
                    </div>
                  </div>
                  <div onClick={(e) => e.stopPropagation()}>
                    <Switch
                      checked={working.enabled}
                      onCheckedChange={(v) => setField(p.name, { enabled: v })}
                    />
                  </div>
                </div>

                {expanded === p.name && (
                  <motion.div
                    initial={{ opacity: 0, height: 0 }}
                    animate={{ opacity: 1, height: "auto" }}
                    transition={springs.gentle}
                    className="px-5 pb-5 pt-1 border-t border-white/5 space-y-3"
                  >
                    <div>
                      <Label>Base URL</Label>
                      <Input
                        value={working.base_url}
                        onChange={(e) => setField(p.name, { base_url: e.target.value })}
                        className="mt-1 font-mono text-xs"
                      />
                    </div>
                    <div>
                      <Label>API key</Label>
                      <Input
                        type="password"
                        value={working.api_key}
                        onChange={(e) => setField(p.name, { api_key: e.target.value })}
                        placeholder="sk-…"
                        className="mt-1 font-mono text-xs"
                      />
                    </div>
                    <div className="grid grid-cols-3 gap-2">
                      <div>
                        <Label>Model</Label>
                        <Input
                          value={working.default_model}
                          onChange={(e) => setField(p.name, { default_model: e.target.value })}
                          className="mt-1 font-mono text-xs"
                        />
                      </div>
                      <div>
                        <Label>Timeout</Label>
                        <Input
                          type="number"
                          value={working.timeout}
                          onChange={(e) => setField(p.name, { timeout: Number(e.target.value) })}
                          className="mt-1 font-mono text-xs"
                        />
                      </div>
                      <div>
                        <Label>Retries</Label>
                        <Input
                          type="number"
                          value={working.max_retries}
                          onChange={(e) => setField(p.name, { max_retries: Number(e.target.value) })}
                          className="mt-1 font-mono text-xs"
                        />
                      </div>
                    </div>
                    <div className="flex gap-2 pt-1">
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => test(p.name)}
                        disabled={testing === p.name}
                        data-cursor="hover"
                      >
                        {testing === p.name ? (
                          <RefreshCw className="h-3 w-3 animate-spin" />
                        ) : (
                          <Zap className="h-3 w-3" />
                        )}
                        Test
                      </Button>
                      <Button
                        variant={isDirty ? "lime" : "primary"}
                        size="sm"
                        onClick={() => save(p.name)}
                        disabled={!isDirty}
                        data-cursor="hover"
                      >
                        <Plug className="h-3 w-3" />
                        Save
                      </Button>
                    </div>
                  </motion.div>
                )}
              </motion.div>
            );
          })}
        </motion.div>

        {filtered.length === 0 && (
          <div className="text-center py-20 text-sm text-[var(--color-ink-muted)]">
            No providers match this filter.
          </div>
        )}
      </div>
    </>
  );
}

function TagIcon({ type }: { type: Filter }) {
  const icons = {
    all: <Plug className="h-3 w-3" />,
    cloud: <Cloud className="h-3 w-3" />,
    local: <Server className="h-3 w-3" />,
    gateway: <Network className="h-3 w-3" />,
  };
  return <span className="inline-flex items-center gap-1">{icons[type]}</span>;
}
