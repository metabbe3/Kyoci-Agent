import { useEffect, useState } from "react";
import { motion, AnimatePresence } from "motion/react";
import { Settings, Check, Loader2, Zap, Brain, Sparkles, Server } from "lucide-react";
import { TopBar } from "@/components/layout/TopBar";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { api, BackendUnreachable } from "@/lib/api";
import type { ProviderConfigDTO } from "@/lib/types";
import { springs } from "@/lib/motion";
import { toast } from "sonner";
import { cn } from "@/lib/utils";

const MASKED = "••••";

export function SettingsPanel() {
  const [configs, setConfigs] = useState<Record<string, ProviderConfigDTO>>({});
  const [selected, setSelected] = useState<string | null>(null);
  const [drafts, setDrafts] = useState<Record<string, ProviderConfigDTO>>({});
  const [testing, setTesting] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api.getConfig()
      .then((r) => {
        setConfigs(r.providers);
        setDrafts(JSON.parse(JSON.stringify(r.providers)));
        const first = Object.keys(r.providers).sort()[0];
        if (first) setSelected(first);
      })
      .catch((e) => toast.error("Load failed: " + e.message));
  }, []);

  const names = Object.keys(configs).sort();
  const cur = selected ? drafts[selected] : null;

  const update = (field: keyof ProviderConfigDTO, value: any) => {
    if (!selected) return;
    setDrafts((d) => ({ ...d, [selected]: { ...d[selected], [field]: value } }));
  };

  const isDirty = () => {
    if (!selected) return false;
    return JSON.stringify(drafts[selected]) !== JSON.stringify(configs[selected]);
  };

  const test = async () => {
    if (!selected) return;
    setTesting(true);
    try {
      if (isDirty()) {
        await api.putConfig({ [selected]: { ...drafts[selected] } });
        setConfigs((c) => ({ ...c, [selected]: { ...drafts[selected] } }));
      }
      const r = await api.testConnection(selected);
      if (r.available) toast.success(`${selected}: connection OK`);
      else toast.error(`${selected}: ${r.error || "unavailable"}`);
    } catch (e: any) {
      if (e instanceof BackendUnreachable) {
        toast.error("Backend unreachable", {
          description: "Start the Go server: `go run ./cmd/server`.",
        });
      } else {
        toast.error(e.message);
      }
    } finally {
      setTesting(false);
    }
  };

  const save = async () => {
    if (!selected) return;
    setSaving(true);
    try {
      const d = drafts[selected];
      await api.putConfig({ [selected]: d });
      setConfigs((c) => ({ ...c, [selected]: { ...d } }));
      toast.success(`${selected}: saved`, {
        description: "Restart the server for changes to take effect.",
      });
    } catch (e: any) {
      if (e instanceof BackendUnreachable) {
        toast.error("Backend unreachable");
      } else {
        toast.error("Save failed: " + e.message);
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <TopBar
        eyebrow="per-provider config"
        title="Settings"
        subtitle="Edit any provider's connection details. Changes save atomically with a .backup."
      />

      <div className="px-6 lg:px-10 pb-16">
        <div className="grid grid-cols-1 lg:grid-cols-[260px_1fr] gap-6">
          {/* Sidebar list */}
          <aside className="glass-panel rounded-2xl p-2 self-start sticky top-32 max-h-[calc(100vh-160px)] overflow-auto">
            <div className="px-3 pt-2 pb-1 text-[10px] font-mono uppercase tracking-[0.22em] text-[var(--color-ink-faint)]">
              {names.length} providers
            </div>
            <ul className="space-y-0.5">
              {names.map((n) => {
                const c = configs[n];
                const active = selected === n;
                return (
                  <li key={n}>
                    <button
                      onClick={() => setSelected(n)}
                      data-cursor="hover"
                      className={cn(
                        "relative w-full text-left px-3 py-2 text-sm font-mono rounded-lg flex items-center justify-between transition-colors",
                        active
                          ? "text-[var(--color-lime)]"
                          : "text-[var(--color-ink-muted)] hover:text-[var(--color-ink)] hover:bg-white/5"
                      )}
                    >
                      {active && (
                        <motion.span
                          layoutId="settings-active"
                          transition={springs.snappy}
                          className="absolute inset-0 rounded-lg"
                          style={{
                            background: "rgba(198, 244, 50, 0.10)",
                            border: "1px solid rgba(198, 244, 50, 0.25)",
                          }}
                        />
                      )}
                      <span className="relative truncate">{n}</span>
                      <span className="relative">
                        {c.enabled ? (
                          <Badge tone="lime">on</Badge>
                        ) : (
                          <Badge tone="outline">off</Badge>
                        )}
                      </span>
                    </button>
                  </li>
                );
              })}
            </ul>
          </aside>

          {/* Detail */}
          <AnimatePresence mode="wait">
            {!cur || !selected ? (
              <motion.p
                key="empty"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
                className="text-sm text-[var(--color-ink-muted)]"
              >
                Pick a provider.
              </motion.p>
            ) : (
              <motion.div
                key={selected}
                initial={{ opacity: 0, y: 12 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -8 }}
                transition={springs.gentle}
                className="glass-panel rounded-2xl p-6 lg:p-8 space-y-5"
              >
                <header>
                  <h2
                    className="text-2xl font-mono font-semibold mb-1"
                    style={{ color: "var(--color-ink)" }}
                  >
                    {selected}
                  </h2>
                  <p className="text-sm text-[var(--color-ink-muted)]">
                    Edits write to <span className="font-mono text-[var(--color-teal)]">config/default.yaml</span>{" "}
                    with a <span className="font-mono text-[var(--color-teal)]">.backup</span>. Restart the
                    server for the changes to take effect at the provider level.
                  </p>
                </header>

                <div className="flex items-center justify-between gap-4 py-2">
                  <div>
                    <Label>Enabled</Label>
                    <p className="text-xs text-[var(--color-ink-muted)] mt-1">
                      Register the provider on next server start.
                    </p>
                  </div>
                  <Switch checked={cur.enabled} onCheckedChange={(v: boolean) => update("enabled", v)} />
                </div>

                <Field label="Base URL">
                  <Input
                    value={cur.base_url}
                    onChange={(e) => update("base_url", e.target.value)}
                    className="font-mono text-xs"
                  />
                </Field>

                <Field label="API Key">
                  <Input
                    type="password"
                    placeholder={cur.api_key === MASKED ? "saved — leave blank to keep" : ""}
                    value={cur.api_key === MASKED ? "" : cur.api_key}
                    onChange={(e) => update("api_key", e.target.value)}
                  />
                  <p className="text-xs text-[var(--color-ink-faint)] mt-1">
                    Never returned by the server. Blank = keep the stored key.
                  </p>
                </Field>

                <Field label="Default model">
                  <Input
                    value={cur.default_model}
                    onChange={(e) => update("default_model", e.target.value)}
                    className="font-mono text-xs"
                  />
                </Field>

                <div className="grid grid-cols-2 gap-3">
                  <Field label="Timeout (s)">
                    <Input
                      type="number"
                      value={cur.timeout}
                      onChange={(e) => update("timeout", parseInt(e.target.value) || 0)}
                    />
                  </Field>
                  <Field label="Max retries">
                    <Input
                      type="number"
                      value={cur.max_retries}
                      onChange={(e) => update("max_retries", parseInt(e.target.value) || 0)}
                    />
                  </Field>
                </div>

                <div className="flex items-center gap-3 pt-3 border-t border-white/5">
                  <Button onClick={save} variant="lime" disabled={!isDirty() || saving} data-cursor="hover">
                    {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />}
                    Save
                  </Button>
                  <Button
                    variant="secondary"
                    onClick={test}
                    disabled={testing || !cur.enabled}
                    data-cursor="hover"
                  >
                    {testing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Zap className="h-4 w-4" />}
                    Test connection
                  </Button>
                  <AnimatePresence>
                    {isDirty() && (
                      <motion.span
                        initial={{ opacity: 0, x: -4 }}
                        animate={{ opacity: 1, x: 0 }}
                        exit={{ opacity: 0, x: -4 }}
                        className="text-xs text-[var(--color-amber)]"
                      >
                        unsaved changes
                      </motion.span>
                    )}
                  </AnimatePresence>
                </div>
              </motion.div>
            )}
          </AnimatePresence>
        </div>

        {/* Generic config info */}
        <div className="mt-8 grid grid-cols-1 md:grid-cols-3 gap-3">
          <InfoTile icon={<Server className="h-4 w-4" />} title="Server" lines={["REST :8080", "GRPC :50051"]} />
          <InfoTile icon={<Brain className="h-4 w-4" />} title="Memory" lines={["SQLite · data/kyoci.db", "Compaction @ 0.75"]} />
          <InfoTile icon={<Sparkles className="h-4 w-4" />} title="Skills" lines={["20 built-in", "Zero-AI fast paths"]} />
        </div>
      </div>
    </>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label>{label}</Label>
      {children}
    </div>
  );
}

function InfoTile({
  icon,
  title,
  lines,
}: {
  icon: React.ReactNode;
  title: string;
  lines: string[];
}) {
  return (
    <div className="glass-panel rounded-2xl p-4 flex items-start gap-3">
      <div className="h-8 w-8 grid place-items-center rounded-lg glass text-[var(--color-lime)] shrink-0">
        {icon}
      </div>
      <div>
        <div className="text-sm font-medium" style={{ fontFamily: "var(--font-display)" }}>
          {title}
        </div>
        <div className="text-xs text-[var(--color-ink-muted)] font-mono mt-0.5 leading-relaxed">
          {lines.map((l, i) => (
            <div key={i}>{l}</div>
          ))}
        </div>
      </div>
    </div>
  );
}
