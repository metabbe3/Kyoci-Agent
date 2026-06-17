import { useEffect, useMemo, useState } from "react";
import { motion, AnimatePresence } from "motion/react";
import * as Dialog from "@radix-ui/react-dialog";
import {
  Search,
  Sparkles,
  Hash,
  Type,
  Binary,
  Calendar,
  Network,
  Lock,
  Palette,
  FileCode,
  Wand2,
  X,
  ArrowRight,
} from "lucide-react";
import { TopBar } from "@/components/layout/TopBar";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import type { SkillInfo } from "@/lib/types";
import { springs, staggerContainer, staggerItem } from "@/lib/motion";
import { cn } from "@/lib/utils";

// Curated metadata for skills that the backend doesn't surface (icon,
// category, example). Falls back to a default for any unmatched skill.
const skillMeta: Record<
  string,
  { category: string; icon: React.ReactNode; accent: string; example: string }
> = {
  math: { category: "Math", icon: <Binary className="h-4 w-4" />, accent: "#5eead4", example: "math 2+2*3" },
  time: { category: "Time", icon: <Calendar className="h-4 w-4" />, accent: "#5eead4", example: "current time" },
  hash: { category: "Security", icon: <Lock className="h-4 w-4" />, accent: "#ff6b5a", example: "sha256 hello" },
  uuid: { category: "Generators", icon: <Sparkles className="h-4 w-4" />, accent: "#a78bfa", example: "generate uuid" },
  encode: { category: "Encoding", icon: <Type className="h-4 w-4" />, accent: "#5eead4", example: "base64 encode hello" },
  convert: { category: "Math", icon: <Binary className="h-4 w-4" />, accent: "#5eead4", example: "100 f to c" },
  color: { category: "Visual", icon: <Palette className="h-4 w-4" />, accent: "#c6f432", example: "#c6f432" },
  regex: { category: "Formatting", icon: <FileCode className="h-4 w-4" />, accent: "#a78bfa", example: "regex /\\d+/ test abc 123" },
  jsonfmt: { category: "Formatting", icon: <FileCode className="h-4 w-4" />, accent: "#a78bfa", example: "format json {\"a\":1}" },
  sqlfmt: { category: "Formatting", icon: <FileCode className="h-4 w-4" />, accent: "#a78bfa", example: "sql select * from users" },
  diff: { category: "Formatting", icon: <FileCode className="h-4 w-4" />, accent: "#a78bfa", example: "diff foo vs foo bar" },
  jwt: { category: "Security", icon: <Lock className="h-4 w-4" />, accent: "#ff6b5a", example: "decode jwt eyJ..." },
  qr: { category: "Generators", icon: <Wand2 className="h-4 w-4" />, accent: "#a78bfa", example: "qr hello" },
  password: { category: "Generators", icon: <Lock className="h-4 w-4" />, accent: "#ff6b5a", example: "password 20" },
  charset: { category: "Encoding", icon: <Type className="h-4 w-4" />, accent: "#5eead4", example: "charset héllo" },
  cron: { category: "Time", icon: <Calendar className="h-4 w-4" />, accent: "#5eead4", example: "cron */5 * * * *" },
  subnet: { category: "Network", icon: <Network className="h-4 w-4" />, accent: "#5eead4", example: "192.168.1.0/24" },
  lorem: { category: "Generators", icon: <Wand2 className="h-4 w-4" />, accent: "#a78bfa", example: "lorem 3 sentences" },
  markdown: { category: "Formatting", icon: <Hash className="h-4 w-4" />, accent: "#a78bfa", example: "markdown a,b,c" },
  emojinfo: { category: "Visual", icon: <Sparkles className="h-4 w-4" />, accent: "#c6f432", example: "emoji 🚀" },
};

const categoryIcon: Record<string, React.ReactNode> = {
  All: <Sparkles className="h-3 w-3" />,
  Math: <Binary className="h-3 w-3" />,
  Time: <Calendar className="h-3 w-3" />,
  Security: <Lock className="h-3 w-3" />,
  Generators: <Wand2 className="h-3 w-3" />,
  Encoding: <Type className="h-3 w-3" />,
  Visual: <Palette className="h-3 w-3" />,
  Formatting: <FileCode className="h-3 w-3" />,
  Network: <Network className="h-3 w-3" />,
};

export function SkillsPanel() {
  const [skills, setSkills] = useState<SkillInfo[]>([]);
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState("All");
  const [selected, setSelected] = useState<SkillInfo | null>(null);

  useEffect(() => {
    api.skills()
      .then((r) => setSkills(r.skills))
      .catch(() => setSkills(fallbackSkills()));
  }, []);

  const categories = useMemo(() => {
    const set = new Set<string>();
    skills.forEach((s) => set.add(skillMeta[s.name]?.category ?? "Other"));
    return ["All", ...Array.from(set).sort()];
  }, [skills]);

  const filtered = useMemo(() => {
    return skills
      .filter((s) => category === "All" || (skillMeta[s.name]?.category ?? "Other") === category)
      .filter(
        (s) =>
          !search ||
          s.name.includes(search.toLowerCase()) ||
          s.description.toLowerCase().includes(search.toLowerCase())
      );
  }, [skills, category, search]);

  return (
    <>
      <TopBar
        eyebrow={`${skills.length} zero-AI shortcuts`}
        title="Skills"
        subtitle="Fast paths that skip the LLM entirely. Math, encoding, parsing — answered in microseconds."
      />

      <div className="px-6 lg:px-10 pb-16">
        {/* Search + categories */}
        <motion.div
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={springs.gentle}
          className="flex flex-col sm:flex-row gap-3 mb-6"
        >
          <div className="relative sm:w-80">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-[var(--color-ink-faint)]" />
            <Input
              placeholder="Search skills…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9"
            />
          </div>
          <div className="flex gap-1 glass-panel rounded-xl p-1 overflow-x-auto">
            {categories.map((c) => (
              <button
                key={c}
                onClick={() => setCategory(c)}
                data-cursor="hover"
                className={cn(
                  "inline-flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg whitespace-nowrap transition-all",
                  category === c
                    ? "bg-[var(--color-lime)] text-[var(--color-void)] font-medium"
                    : "text-[var(--color-ink-muted)] hover:text-[var(--color-ink)]"
                )}
              >
                {categoryIcon[c]}
                {c}
              </button>
            ))}
          </div>
        </motion.div>

        {/* Skills grid */}
        <motion.div
          initial="hidden"
          animate="visible"
          variants={staggerContainer(0.05, 0.04)}
          className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4"
        >
          {filtered.map((s) => {
            const meta = skillMeta[s.name] ?? {
              category: "Other",
              icon: <Sparkles className="h-4 w-4" />,
              accent: "#9ba1ad",
              example: s.name,
            };
            return (
              <motion.button
                key={s.name}
                variants={staggerItem}
                transition={springs.gentle}
                whileHover={{ y: -4 }}
                onClick={() => setSelected(s)}
                data-cursor="hover"
                className="glass-panel rounded-2xl p-5 text-left relative overflow-hidden group"
              >
                <div
                  aria-hidden
                  className="absolute -top-10 -right-10 h-28 w-28 rounded-full opacity-15 blur-3xl pointer-events-none transition-opacity duration-500 group-hover:opacity-40"
                  style={{ background: meta.accent }}
                />
                <div className="relative">
                  <div
                    className="h-10 w-10 rounded-xl grid place-items-center mb-3"
                    style={{
                      background: `${meta.accent}1a`,
                      border: `1px solid ${meta.accent}40`,
                      color: meta.accent,
                    }}
                  >
                    {meta.icon}
                  </div>
                  <div className="flex items-center gap-2 mb-1">
                    <h3
                      className="text-base font-semibold font-mono"
                      style={{ color: "var(--color-ink)" }}
                    >
                      {s.name}
                    </h3>
                  </div>
                  <p className="text-xs text-[var(--color-ink-muted)] leading-relaxed line-clamp-2 mb-3">
                    {s.description}
                  </p>
                  <div className="flex items-center justify-between">
                    <Badge tone="outline">{meta.category}</Badge>
                    <ArrowRight
                      className="h-3 w-3 text-[var(--color-ink-faint)] opacity-0 group-hover:opacity-100 transition-opacity"
                      style={{ color: meta.accent }}
                    />
                  </div>
                </div>
              </motion.button>
            );
          })}
        </motion.div>

        {filtered.length === 0 && (
          <div className="text-center py-20 text-sm text-[var(--color-ink-muted)]">
            No skills match this filter.
          </div>
        )}

        {/* Footer note */}
        <div className="mt-10 glass-panel rounded-2xl p-5 flex items-start gap-3">
          <div
            className="h-8 w-8 rounded-lg grid place-items-center shrink-0"
            style={{
              background: "rgba(198, 244, 50, 0.12)",
              border: "1px solid rgba(198, 244, 50, 0.3)",
              color: "var(--color-lime)",
            }}
          >
            <Sparkles className="h-4 w-4" />
          </div>
          <div>
            <p className="text-sm text-[var(--color-ink)] font-medium">Zero-AI fast paths</p>
            <p className="text-xs text-[var(--color-ink-muted)] mt-1 leading-relaxed">
              When a user query matches a skill, the orchestrator short-circuits the LLM
              and runs deterministic Go code instead. Faster, cheaper, and 100% reproducible.
              Try one in <span className="font-mono text-[var(--color-teal)]">Chat → Agent</span> mode.
            </p>
          </div>
        </div>
      </div>

      {/* Skill detail modal */}
      <Dialog.Root open={!!selected} onOpenChange={(o) => !o && setSelected(null)}>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out" />
          <Dialog.Content className="fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2 w-[90vw] max-w-lg">
            <AnimatePresence>
              {selected && (
                <motion.div
                  initial={{ opacity: 0, scale: 0.95, y: 8 }}
                  animate={{ opacity: 1, scale: 1, y: 0 }}
                  exit={{ opacity: 0, scale: 0.95 }}
                  transition={springs.snappy}
                  className="glass-panel rounded-2xl p-6"
                >
                  {(() => {
                    const meta = skillMeta[selected.name] ?? {
                      category: "Other",
                      icon: <Sparkles className="h-4 w-4" />,
                      accent: "#9ba1ad",
                      example: selected.name,
                    };
                    return (
                      <>
                        <div className="flex items-start gap-4 mb-4">
                          <div
                            className="h-12 w-12 rounded-xl grid place-items-center shrink-0"
                            style={{
                              background: `${meta.accent}1a`,
                              border: `1px solid ${meta.accent}40`,
                              color: meta.accent,
                            }}
                          >
                            {meta.icon}
                          </div>
                          <div className="flex-1">
                            <Dialog.Title
                              className="text-2xl font-semibold font-mono"
                              style={{ color: "var(--color-ink)" }}
                            >
                              {selected.name}
                            </Dialog.Title>
                            <Dialog.Description className="text-sm text-[var(--color-ink-muted)] mt-1">
                              {selected.description}
                            </Dialog.Description>
                          </div>
                          <Dialog.Close asChild>
                            <button
                              data-cursor="hover"
                              className="h-8 w-8 grid place-items-center rounded-lg glass hover:bg-white/10"
                            >
                              <X className="h-4 w-4" />
                            </button>
                          </Dialog.Close>
                        </div>
                        <div className="flex flex-wrap gap-2 mb-4">
                          <Badge tone="lime">{meta.category}</Badge>
                          {(selected.keywords ?? []).slice(0, 6).map((k) => (
                            <Badge key={k} tone="outline">
                              {k}
                            </Badge>
                          ))}
                        </div>
                        <div className="rounded-xl border border-white/10 bg-black/30 p-3 font-mono text-xs">
                          <div className="text-[10px] uppercase tracking-wider text-[var(--color-ink-faint)] mb-1">
                            Try
                          </div>
                          <div className="text-[var(--color-teal)]">{meta.example}</div>
                        </div>
                      </>
                    );
                  })()}
                </motion.div>
              )}
            </AnimatePresence>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </>
  );
}

function fallbackSkills(): SkillInfo[] {
  return [
    { name: "math", description: "Evaluate mathematical expressions", keywords: ["math"] },
    { name: "time", description: "Current time and date", keywords: ["time"] },
    { name: "hash", description: "MD5, SHA1, SHA256", keywords: ["hash"] },
    { name: "uuid", description: "Generate UUIDs", keywords: ["uuid"] },
    { name: "encode", description: "Base64 and URL encoding", keywords: ["encode"] },
    { name: "convert", description: "Unit conversions", keywords: ["convert"] },
  ];
}
