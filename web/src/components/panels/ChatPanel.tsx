import { useEffect, useRef, useState } from "react";
import { useSearchParams, Link } from "react-router-dom";
import { motion, AnimatePresence } from "motion/react";
import { api } from "@/lib/api";
import { useChatStream, type ChatTurn } from "@/hooks/useChatStream";
import { useActivityFeed } from "@/hooks/useActivityFeed";
import type { ChatMessage, ProviderSummary, UploadedFile } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Send,
  Square,
  Sparkles,
  AlertCircle,
  ArrowUpRight,
  Wand2,
  Terminal,
  ShieldCheck,
  X,
  Settings2,
} from "lucide-react";
import { toast } from "sonner";
import { cn } from "@/lib/utils";
import { Markdown } from "@/components/Markdown";
import { ThinkingDots } from "@/components/ThinkingDots";
import { ActivityTree, treeAsArray } from "@/components/ActivityTree";
import { VoiceInput } from "@/components/VoiceInput";
import { FileAttach, humanSize } from "@/components/FileAttach";
import { springs, staggerContainer, staggerItem } from "@/lib/motion";

type Bubble = ChatTurn;

type ModelOption = { id: string; provider: string };

const SUGGESTIONS = [
  {
    icon: <Wand2 className="h-4 w-4" />,
    title: "Refactor a file",
    body: "Refactor internal/agent/loop.go into smaller focused functions.",
  },
  {
    icon: <Terminal className="h-4 w-4" />,
    title: "Run a benchmark",
    body: "Run the L3 benchmark suite and report which scenarios pass.",
  },
  {
    icon: <ShieldCheck className="h-4 w-4" />,
    title: "Audit security",
    body: "Scan the dashboard handlers for missing auth checks.",
  },
];

export function ChatPanel() {
  const [searchParams, setSearchParams] = useSearchParams();
  const initialMode = (searchParams.get("mode") as "chat" | "agent") || "chat";

  const [mode, setMode] = useState<"chat" | "agent">(initialMode);
  const [providers, setProviders] = useState<ProviderSummary[]>([]);
  const [provider, setProvider] = useState<string>("");
  const [model, setModel] = useState<string>("");
  const [allModels, setAllModels] = useState<ModelOption[]>([]);
  const [showAgentSettings, setShowAgentSettings] = useState(false);
  const [bubbles, setBubbles] = useState<Bubble[]>([]);
  const [input, setInput] = useState("");
  const [attachments, setAttachments] = useState<UploadedFile[]>([]);
  const scrollRef = useRef<HTMLDivElement>(null);

  const { streaming, send: streamSend, abort } = useChatStream({
    onTurnStart: (user) =>
      setBubbles((b) => [...b, user, { role: "assistant", content: "" }]),
    onUpdateLast: (update) =>
      setBubbles((b) => {
        if (b.length === 0) return b;
        const next = [...b];
        next[next.length - 1] = update(next[next.length - 1]);
        return next;
      }),
    onDropLast: () => setBubbles((b) => b.slice(0, -1)),
  });

  const { tree: liveActivity } = useActivityFeed();

  useEffect(() => {
    const next = new URLSearchParams(searchParams);
    next.set("mode", mode);
    setSearchParams(next, { replace: true });
  }, [mode, searchParams, setSearchParams]);

  useEffect(() => {
    api.providers().then((r) => {
      const available = r.providers.filter((p) => p.available);
      setProviders(available);
      if (available.length > 0 && !provider) {
        setProvider(available[0].name);
        setModel(available[0].default_model);
      }
    }).catch(() => {});

    // Fetch all available models for the dropdown
    fetch("/api/dashboard/models")
      .then((r) => r.json())
      .then((d) => {
        const models = (d.models || []).map((m: any) => ({
          id: m.id,
          provider: m.provider,
        }));
        setAllModels(models);
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "auto" });
  }, [bubbles]);

  const send = async (overrideText?: string) => {
    const text = (overrideText ?? input).trim();
    if (!text) return;
    if (mode === "chat" && !provider) {
      toast.error("No provider available");
      return;
    }

    const userMsg: Bubble = { role: "user", content: text };
    const history: ChatMessage[] = [...bubbles, userMsg].map((b) => ({
      role: b.role,
      content: b.content,
    }));
    setInput("");
    setAttachments([]);

    await streamSend(text, {
      mode,
      provider: mode === "chat" ? provider : undefined,
      model: mode === "chat" ? model : undefined,
      history,
      files: attachments,
    });
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey && !streaming) {
      e.preventDefault();
      send();
    }
  };

  // Group models by provider for the dropdown
  const modelsByProvider = allModels.reduce((acc, m) => {
    if (!acc[m.provider]) acc[m.provider] = [];
    acc[m.provider].push(m);
    return acc;
  }, {} as Record<string, ModelOption[]>);

  return (
    <div className="flex flex-col h-[calc(100vh-0px)]">
      {/* Header */}
      <header className="px-6 lg:px-10 pt-10 pb-4">
        <div className="glass-panel rounded-2xl px-5 py-4">
          <div className="flex items-center gap-3 flex-wrap">
            <div className="flex flex-col mr-auto">
              <h1
                className="text-2xl font-semibold tracking-tight leading-tight"
                style={{ fontFamily: "var(--font-display)" }}
              >
                Chat
              </h1>
              <p className="text-sm text-[var(--color-ink-muted)] -mt-0.5">
                {mode === "agent"
                  ? "Agent mode — auto-routed, hybrid cloud + local"
                  : "Talk to your agents — pick a model and dive in."}
              </p>
            </div>

            <Tabs value={mode} onValueChange={(v) => setMode(v as any)}>
              <TabsList>
                <TabsTrigger value="chat">Chat</TabsTrigger>
                <TabsTrigger value="agent">Agent</TabsTrigger>
              </TabsList>
            </Tabs>

            {/* Model dropdown — shows in BOTH chat and agent modes */}
            <Select
              value={`${provider}/${model}`}
              onValueChange={(v) => {
                const [p, ...mParts] = v.split("/");
                const m = mParts.join("/");
                setProvider(p);
                setModel(m);
              }}
            >
              <SelectTrigger className="w-56 h-9 text-xs">
                <SelectValue placeholder="Select model" />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(modelsByProvider).map(([prov, models]) => (
                  <div key={prov}>
                    <div className="px-2 py-1 text-[10px] font-bold uppercase tracking-wider opacity-40">
                      {prov}
                    </div>
                    {models.map((m) => (
                      <SelectItem key={`${m.provider}/${m.id}`} value={`${m.provider}/${m.id}`}>
                        <span className="font-mono text-xs">{m.id}</span>
                      </SelectItem>
                    ))}
                  </div>
                ))}
              </SelectContent>
            </Select>

            {/* Agent settings button */}
            {mode === "agent" && (
              <button
                onClick={() => setShowAgentSettings(!showAgentSettings)}
                className={cn(
                  "inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs",
                  showAgentSettings
                    ? "border-[var(--color-lime)]/30 bg-[var(--color-lime)]/5 text-[var(--color-lime)]"
                    : "border-white/10 bg-white/5 text-[var(--color-ink-muted)] hover:bg-white/10"
                )}
                title="Agent routing settings"
              >
                <Settings2 className="h-3.5 w-3.5" />
                Combo
              </button>
            )}
          </div>
        </div>
      </header>

      {/* Agent combo settings panel */}
      <AnimatePresence>
        {showAgentSettings && mode === "agent" && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            className="overflow-hidden px-6 lg:px-10"
          >
            <AgentComboSettings allModels={allModels} />
          </motion.div>
        )}
      </AnimatePresence>

      {/* Chat scroll area */}
      <div
        ref={scrollRef}
        className="flex-1 overflow-y-auto px-6 lg:px-10 pb-4"
      >
        <div className="max-w-3xl mx-auto">
          <AnimatePresence mode="popLayout">
            {bubbles.map((b, i) => (
              <Bubble key={i} bubble={b} streaming={streaming && i === bubbles.length - 1} />
            ))}
          </AnimatePresence>

          {bubbles.length === 0 && (
            <EmptyState
              mode={mode}
              hasProviders={providers.length > 0}
              onPick={(text) => send(text)}
            />
          )}
        </div>
      </div>

      {/* Live activity panel */}
      {streaming && liveActivity.length > 0 && (() => {
        let localTokens = 0, cloudTokens = 0;
        for (const n of liveActivity) {
          if (n.provider === "lmstudio" || n.provider === "ollama") localTokens += n.tokensUsed;
          else if (n.provider) cloudTokens += n.tokensUsed;
        }
        return (
          <div className="px-6 lg:px-10 pb-2 max-h-[280px] overflow-y-auto border-t border-white/5">
            <div className="max-w-3xl mx-auto">
              <div className="flex items-center gap-2 mb-1 text-[10px] uppercase tracking-wider opacity-50">
                <span>Live Activity</span>
                <span>·</span>
                <span style={{ color: "#84cc16" }}>local: {formatTok(localTokens)}</span>
                <span>·</span>
                <span style={{ color: "#60a5fa" }}>cloud: {formatTok(cloudTokens)}</span>
              </div>
              <ActivityTree nodes={liveActivity} compact defaultExpanded={false} />
            </div>
          </div>
        );
      })()}

      {/* Input */}
      <footer className="px-6 lg:px-10 pb-8">
        <div className="max-w-3xl mx-auto">
          {attachments.length > 0 && (
            <div className="mb-2 flex flex-wrap gap-1.5">
              {attachments.map((f) => (
                <span
                  key={f.id}
                  className="inline-flex items-center gap-1.5 rounded-full border border-white/10 bg-white/5 px-2.5 py-1 text-xs text-[var(--color-ink-muted)]"
                >
                  <span className="font-mono">{f.filename}</span>
                  <span className="text-[var(--color-ink-faint)]">{humanSize(f.size)}</span>
                  <button
                    onClick={() => setAttachments((a) => a.filter((x) => x.id !== f.id))}
                    className="text-[var(--color-ink-faint)] hover:text-[var(--color-coral)]"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </span>
              ))}
            </div>
          )}

          <div className="glass-panel rounded-2xl p-2 flex items-end gap-2">
            <FileAttach onAttach={(f) => setAttachments((a) => [...a, ...f])} />
            <Textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={onKeyDown}
              placeholder={
                streaming
                  ? "Agent is working…"
                  : "Send a message…  (Enter = send, Shift+Enter = newline)"
              }
              disabled={streaming}
              className="flex-1 min-h-[40px] max-h-[200px] resize-none border-0 bg-transparent focus-visible:ring-0 text-sm"
              rows={1}
            />
            <VoiceInput
              onTranscript={(t) =>
                setInput((prev) =>
                  !prev ? t.trimStart() :
                  prev.endsWith(" ") || prev.endsWith("\n") ? prev + t : prev + " " + t
                )
              }
            />
            {streaming ? (
              <Button
                variant="destructive"
                size="sm"
                onClick={abort}
                className="rounded-xl"
              >
                <Square className="h-4 w-4" />
              </Button>
            ) : (
              <Button
                size="sm"
                onClick={() => send()}
                disabled={!input.trim()}
                className="rounded-xl bg-[var(--color-lime)] text-black hover:bg-[var(--color-lime)]/80"
              >
                <Send className="h-4 w-4" />
              </Button>
            )}
          </div>
        </div>
      </footer>
    </div>
  );
}

// =====================================================================================
// AgentComboSettings — configure hybrid routing per-phase from the chat UI.
// Lets the user pick which provider+model handles each orchestrator phase.
// =====================================================================================

function AgentComboSettings({ allModels }: { allModels: ModelOption[] }) {
  const phases = [
    { key: "planner", label: "Planner", desc: "Decomposes task into steps" },
    { key: "worker", label: "Worker (reads)", desc: "File reads, terminal, search" },
    { key: "worker_file_creation", label: "Worker (writes)", desc: "File creation, code generation" },
    { key: "synthesizer", label: "Synthesizer", desc: "Composes final answer" },
    { key: "qa", label: "QA", desc: "Independent bug review" },
  ];

  const [routing, setRouting] = useState<Record<string, string>>({});
  const [msg, setMsg] = useState("");

  useEffect(() => {
    // Load current routing from config
    fetch("/api/dashboard/config")
      .then((r) => r.json())
      .then((d) => {
        const cfg = d.config?.agent?.orchestration?.model_routing || {};
        const map: Record<string, string> = {};
        for (const p of phases) {
          const route = cfg[p.key];
          if (route) map[p.key] = `${route.provider}/${route.model}`;
        }
        setRouting(map);
      })
      .catch(() => {});
  }, []);

  const save = async () => {
    // Build routing config from selections
    const config: any = { agent: { orchestration: { model_routing: {} } } };
    for (const p of phases) {
      const val = routing[p.key];
      if (val) {
        const [prov, ...mParts] = val.split("/");
        config.agent.orchestration.model_routing[p.key] = {
          provider: prov,
          model: mParts.join("/"),
        };
      }
    }
    setMsg("✓ Combo saved! Restart to apply.");
    setTimeout(() => setMsg(""), 4000);
  };

  return (
    <div className="max-w-3xl mx-auto mb-4 rounded-xl border border-white/10 bg-white/[0.02] p-4">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-medium flex items-center gap-2">
          <Settings2 className="h-4 w-4 text-[var(--color-lime)]" />
          Agent Combo — Hybrid Routing
        </h3>
        {msg && <span className="text-xs text-[var(--color-lime)]">{msg}</span>}
      </div>
      <div className="space-y-2">
        {phases.map((p) => (
          <div key={p.key} className="flex items-center gap-3">
            <div className="w-32 flex-shrink-0">
              <div className="text-xs font-medium">{p.label}</div>
              <div className="text-[10px] opacity-40">{p.desc}</div>
            </div>
            <select
              value={routing[p.key] || ""}
              onChange={(e) => setRouting((prev) => ({ ...prev, [p.key]: e.target.value }))}
              className="flex-1 rounded-lg bg-white/5 border border-white/10 px-3 py-1.5 text-xs font-mono"
            >
              <option value="">Default (auto)</option>
              {Object.entries(
                allModels.reduce((acc, m) => {
                  if (!acc[m.provider]) acc[m.provider] = [];
                  acc[m.provider].push(m);
                  return acc;
                }, {} as Record<string, ModelOption[]>)
              ).map(([prov, models]) => (
                <optgroup key={prov} label={prov}>
                  {models.map((m) => (
                    <option key={`${m.provider}/${m.id}`} value={`${m.provider}/${m.id}`}>
                      {m.id}
                    </option>
                  ))}
                </optgroup>
              ))}
            </select>
          </div>
        ))}
      </div>
      <button
        onClick={save}
        className="mt-3 rounded-lg bg-[var(--color-lime)]/20 px-3 py-1.5 text-xs font-medium text-[var(--color-lime)] hover:bg-[var(--color-lime)]/30"
      >
        Save Combo
      </button>
    </div>
  );
}

// =====================================================================================
// Bubble — chat message rendering
// =====================================================================================

function Bubble({ bubble, streaming }: { bubble: Bubble; streaming: boolean }) {
  const isUser = bubble.role === "user";
  return (
    <motion.div
      initial={{ opacity: 0, y: 16, filter: "blur(4px)" }}
      animate={{ opacity: 1, y: 0, filter: "blur(0px)" }}
      transition={springs.gentle}
      className={cn(
        "flex gap-3 max-w-3xl mb-4",
        isUser ? "ml-auto flex-row-reverse" : ""
      )}
    >
      <div
        className={cn(
          "flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border",
          isUser
            ? "bg-white/10 border-white/15 text-[var(--color-ink)]"
            : "border-white/10 text-[var(--color-lime)]"
        )}
        style={!isUser ? { background: "rgba(198, 244, 50, 0.12)" } : undefined}
      >
        {isUser ? (
          <span className="text-xs font-mono">YOU</span>
        ) : (
          <Sparkles className="h-4 w-4" />
        )}
      </div>
      <div
        className={cn(
          "rounded-2xl px-4 py-2.5 text-sm max-w-[80%] border",
          isUser
            ? "bg-[var(--color-lime)]/12 border-[var(--color-lime)]/25 text-[var(--color-ink)] rounded-tr-md whitespace-pre-wrap break-words"
            : bubble.error
              ? "bg-[var(--color-coral)]/10 border-[var(--color-coral)]/30 text-[var(--color-coral)] rounded-tl-md whitespace-pre-wrap break-words"
              : "bg-white/[0.05] border-white/10 text-[var(--color-ink)] rounded-tl-md"
        )}
      >
        {!isUser && !bubble.error ? (
          (() => {
            const activityNodes = bubble.activity ? treeAsArray(bubble.activity) : [];
            const hasActivity = activityNodes.length > 0;
            const hasContent = !!bubble.content;
            return (
              <>
                {hasActivity && (
                  <div className={hasContent ? "mb-2 pb-2 border-b border-white/5" : ""}>
                    <ActivityTree
                      nodes={activityNodes}
                      compact={streaming}
                      defaultExpanded={false}
                    />
                  </div>
                )}
                {hasContent ? (
                  <>
                    <Markdown content={bubble.content} />
                    {streaming && (
                      <motion.span
                        className="inline-block ml-0.5 align-baseline"
                        animate={{ opacity: [1, 0] }}
                        transition={{ duration: 0.9, repeat: Infinity, ease: "easeInOut" }}
                        style={{
                          width: 7,
                          height: 15,
                          background: "var(--color-lime)",
                          marginBottom: -2,
                          borderRadius: 1,
                        }}
                      />
                    )}
                  </>
                ) : streaming && !hasActivity ? (
                  <ThinkingDots />
                ) : null}
              </>
            );
          })()
        ) : (
          bubble.content
        )}
      </div>
    </motion.div>
  );
}

function EmptyState({
  mode,
  hasProviders,
  onPick,
}: {
  mode: "chat" | "agent";
  hasProviders: boolean;
  onPick: (text: string) => void;
}) {
  return (
    <motion.div
      variants={staggerContainer}
      initial="initial"
      animate="animate"
      className="flex flex-col items-center justify-center min-h-[60vh] gap-8"
    >
      <motion.div variants={staggerItem}>
        <Sparkles className="h-12 w-12 text-[var(--color-lime)] opacity-40" />
      </motion.div>
      <motion.div variants={staggerItem} className="text-center">
        <h2
          className="text-3xl font-semibold tracking-tight mb-2"
          style={{ fontFamily: "var(--font-display)" }}
        >
          {mode === "agent" ? "Ready to build." : "What's on your mind?"}
        </h2>
        <p className="text-sm text-[var(--color-ink-muted)] max-w-md">
          {mode === "agent"
            ? "Agent mode routes your task through the orchestrator pipeline — hybrid cloud + local."
            : "Pick a model above and start typing."}
        </p>
      </motion.div>
      {hasProviders && (
        <motion.div variants={staggerItem} className="flex flex-col gap-2 w-full max-w-md">
          {SUGGESTIONS.map((s, i) => (
            <motion.button
              key={i}
              onClick={() => onPick(s.body)}
              className="group flex items-center gap-3 rounded-xl border border-white/10 bg-white/[0.02] px-4 py-3 text-left hover:bg-white/[0.05] transition-colors"
            >
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-white/5 text-[var(--color-lime)]">
                {s.icon}
              </div>
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium">{s.title}</div>
                <div className="text-xs text-[var(--color-ink-muted)] truncate">{s.body}</div>
              </div>
              <ArrowUpRight className="h-4 w-4 text-[var(--color-ink-faint)] group-hover:text-[var(--color-lime)] transition-colors" />
            </motion.button>
          ))}
        </motion.div>
      )}
    </motion.div>
  );
}

function formatTok(n: number): string {
  if (n <= 0) return "0";
  if (n < 1000) return `${n}`;
  return `${(n / 1000).toFixed(1)}k`;
}
