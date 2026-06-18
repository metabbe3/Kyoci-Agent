import { useEffect, useState } from "react";
import { motion } from "motion/react";
import { Cpu, HardDrive, Monitor, AlertTriangle, Cloud, Zap } from "lucide-react";
import { TopBar } from "@/components/layout/TopBar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { api, BackendUnreachable } from "@/lib/api";
import { toastApiError } from "@/lib/api/toast";
import type { HardwareSpecs, RecommendResult, RecommendPick } from "@/lib/types";
import { springs, staggerContainer, staggerItem } from "@/lib/motion";
import { toast } from "sonner";

export function HardwarePanel() {
  const [specs, setSpecs] = useState<HardwareSpecs | null>(null);
  const [recs, setRecs] = useState<RecommendResult | null>(null);
  const [config, setConfig] = useState<Record<string, any> | null>(null);

  const refresh = () => {
    api.hardware().then(setSpecs).catch(() => {});
    api.recommendations().then(setRecs).catch(() => {});
    api.getConfig().then((r) => setConfig(r.providers as any)).catch(() => {});
  };
  useEffect(refresh, []);

  const setOllamaDefault = async (model: string) => {
    try {
      const cur = config?.ollama;
      if (!cur) throw new Error("ollama provider not in config");
      await api.putConfig({
        ollama: { ...cur, default_model: model, api_key: "••••" },
      });
      toast.success(`Ollama default → ${model}`, {
        description: "Restart the server to apply.",
      });
      refresh();
    } catch (e) {
      // toastApiError already maps BackendUnreachable → the actionable hint,
      // so the instanceof branch is no longer needed. Keep the import for any
      // future callers that still branch on it.
      void BackendUnreachable;
      toastApiError(e, { action: `Set Ollama default → ${model}` });
    }
  };

  return (
    <>
      <TopBar
        eyebrow="auto-detected"
        title="Hardware"
        subtitle="What this host can actually run. Used to size local model picks."
      />

      <div className="px-6 lg:px-10 pb-16 max-w-5xl">
        {specs && (
          <motion.div
            initial="hidden"
            animate="visible"
            variants={staggerContainer(0.05, 0.06)}
            className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-8"
          >
            <SpecCard
              icon={<Cpu className="h-4 w-4" />}
              label="CPU"
              delay={0}
              big={`${specs.cpu_count} cores`}
              small={specs.chip_model || specs.arch}
            />
            <SpecCard
              icon={<HardDrive className="h-4 w-4" />}
              label="Memory"
              delay={0.05}
              big={`${specs.ram_gb} GB`}
              small={specs.is_apple_silicon ? "Apple Silicon · unified" : "System RAM"}
            />
            <SpecCard
              icon={<Monitor className="h-4 w-4" />}
              label="GPU"
              delay={0.1}
              big={specs.gpu_model || (specs.is_apple_silicon ? "Apple GPU" : "—")}
              small={
                specs.vram_gb > 0
                  ? `${specs.vram_gb} GB VRAM`
                  : specs.is_apple_silicon
                    ? "shared with RAM"
                    : "no NVIDIA GPU"
              }
            />
          </motion.div>
        )}

        {specs?.warnings && specs.warnings.length > 0 && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={springs.gentle}
            className="glass-panel rounded-2xl p-5 mb-6"
            style={{ borderColor: "rgba(251, 191, 36, 0.25)" }}
          >
            <div className="flex items-start gap-3">
              <div
                className="h-8 w-8 grid place-items-center rounded-lg shrink-0"
                style={{
                  background: "rgba(251, 191, 36, 0.12)",
                  border: "1px solid rgba(251, 191, 36, 0.3)",
                  color: "var(--color-amber)",
                }}
              >
                <AlertTriangle className="h-4 w-4" />
              </div>
              <div>
                <p className="text-sm font-medium text-[var(--color-amber)]">Detection warnings</p>
                <ul className="text-xs text-[var(--color-ink-muted)] mt-1 space-y-0.5">
                  {specs.warnings.map((w, i) => (
                    <li key={i}>· {w}</li>
                  ))}
                </ul>
              </div>
            </div>
          </motion.div>
        )}

        {recs && (
          <>
            <SectionLabel>Recommended local models</SectionLabel>
            <motion.p
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              className="text-sm text-[var(--color-ink-muted)] mb-3"
            >
              {recs.summary}
            </motion.p>
            <motion.div
              initial="hidden"
              animate="visible"
              variants={staggerContainer(0.05, 0.05)}
              className="space-y-2 mb-8"
            >
              {recs.local.map((p, i) => (
                <PickRow key={p.model} pick={p} onSet={setOllamaDefault} delay={i * 0.03} />
              ))}
            </motion.div>

            <SectionLabel>
              <Cloud className="h-3 w-3 inline mr-1" />
              Cloud guidance
            </SectionLabel>
            <motion.div
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              transition={springs.gentle}
              className="glass-panel rounded-2xl p-5"
            >
              <div className="flex items-center justify-between gap-3 mb-3">
                <p className="text-sm text-[var(--color-ink-muted)]">{recs.cloud.summary}</p>
                <Badge tone={recs.cloud.needed ? "warning" : "lime"}>
                  {recs.cloud.needed ? "use cloud" : "local is fine"}
                </Badge>
              </div>
              {recs.cloud.recommended_providers && recs.cloud.recommended_providers.length > 0 && (
                <div className="flex flex-wrap gap-1.5">
                  {recs.cloud.recommended_providers.map((p) => (
                    <Badge key={p} tone="outline" className="font-mono">
                      {p}
                    </Badge>
                  ))}
                </div>
              )}
            </motion.div>
          </>
        )}
      </div>
    </>
  );
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <h2
      className="text-xl font-semibold tracking-tight mb-2"
      style={{ fontFamily: "var(--font-display)" }}
    >
      {children}
    </h2>
  );
}

function SpecCard({
  icon,
  label,
  big,
  small,
  delay = 0,
}: {
  icon: React.ReactNode;
  label: string;
  big: string;
  small: string;
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
        className="text-2xl lg:text-3xl font-semibold leading-tight tracking-tight"
        style={{ fontFamily: "var(--font-display)" }}
      >
        {big}
      </div>
      <div className="text-xs font-mono text-[var(--color-ink-faint)] mt-1 truncate">
        {small}
      </div>
    </motion.div>
  );
}

function PickRow({
  pick,
  onSet,
  delay = 0,
}: {
  pick: RecommendPick;
  onSet: (m: string) => void;
  delay?: number;
}) {
  const tone = {
    fits: "lime",
    tight: "warning",
    slow: "warning",
    too_big: "destructive",
  }[pick.verdict] as any;

  return (
    <motion.div
      variants={staggerItem}
      transition={{ ...springs.gentle, delay }}
      className="glass-panel rounded-xl px-4 py-3 flex items-center gap-4"
    >
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="font-mono text-sm font-medium">{pick.model}</span>
          {pick.recommended && (
            <Badge tone="lime">
              <Zap className="h-3 w-3" />
              best fit
            </Badge>
          )}
          <Badge tone={tone}>{pick.verdict}</Badge>
        </div>
        <p className="text-xs text-[var(--color-ink-muted)] mt-1">
          {pick.reason}
          {pick.context_len > 0 && (
            <> · {`${(pick.context_len / 1000).toFixed(0)}k context`}</>
          )}
        </p>
      </div>
      {pick.verdict !== "too_big" && (
        <Button size="sm" variant="secondary" onClick={() => onSet(pick.model)} data-cursor="hover">
          Set as Ollama default
        </Button>
      )}
    </motion.div>
  );
}
