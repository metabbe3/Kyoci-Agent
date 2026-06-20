import { useEffect, useRef, useState } from "react";
import { useSearchParams, Link } from "react-router-dom";
import { motion, AnimatePresence } from "motion/react";
import { api } from "@/lib/api";
import { useChatStream, type ChatTurn } from "@/hooks/useChatStream";
import type { ChatMessage, ProviderSummary, UploadedFile } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { RoleBadge, type Role, roleAccents } from "@/components/ui/RoleBadge";
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

const ROLES: { role: Role; abbr: string }[] = [
  { role: "generalist", abbr: "GEN" },
  { role: "developer", abbr: "DEV" },
  { role: "frontend", abbr: "UX" },
  { role: "qa", abbr: "QA" },
  { role: "sre", abbr: "SRE" },
  { role: "pm", abbr: "PM" },
];

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
  const initialRole = (searchParams.get("role") as Role) || "generalist";

  const [mode, setMode] = useState<"chat" | "agent">(initialMode);
  const [role, setRole] = useState<Role>(initialRole);
  const [providers, setProviders] = useState<ProviderSummary[]>([]);
  const [provider, setProvider] = useState<string>("");
  const [model, setModel] = useState<string>("");
  const [bubbles, setBubbles] = useState<Bubble[]>([]);
  const [input, setInput] = useState("");
  const [attachments, setAttachments] = useState<UploadedFile[]>([]);
  const scrollRef = useRef<HTMLDivElement>(null);

  // Streaming lifecycle (abort controller, SSE loop, error mapping) lives in
  // the hook; the panel only supplies how to mutate its bubbles.
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

  // Sync URL when mode/role change (so user can share/bookmark specific states)
  useEffect(() => {
    const next = new URLSearchParams(searchParams);
    next.set("mode", mode);
    if (mode === "agent") next.set("role", role);
    else next.delete("role");
    setSearchParams(next, { replace: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, role]);

  useEffect(() => {
    api.providers().then((r) => {
      const available = r.providers.filter((p) => p.available);
      setProviders(available);
      if (available.length > 0 && !provider) {
        setProvider(available[0].name);
        setModel(available[0].default_model);
      }
    }).catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "auto" });
  }, [bubbles]);

  const send = async (overrideText?: string) => {
    const text = (overrideText ?? input).trim();
    if (!text) return;
    if (mode === "chat" && !provider) {
      toast.error("No provider available", {
        description: "Enable one in Providers, then restart the server.",
      });
      return;
    }

    const userMsg: Bubble = { role: "user", content: text };
    // History includes everything said so far plus this new user turn. Built
    // before clearing input/attachments so the request reflects what was sent.
    const history: ChatMessage[] = [...bubbles, userMsg].map((b) => ({
      role: b.role,
      content: b.content,
    }));
    setInput("");
    setAttachments([]);

    await streamSend(text, {
      mode,
      provider,
      model,
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

  const activeProvider = providers.find((p) => p.name === provider);
  const roleAccent = roleAccents[role];

  return (
    <div className="flex flex-col h-[calc(100vh-0px)]">
      {/* Header — glass pill */}
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
                Talk to your agents — pick a mode and dive in.
              </p>
            </div>
            <Tabs value={mode} onValueChange={(v) => setMode(v as any)}>
              <TabsList>
                <TabsTrigger value="chat">Chat</TabsTrigger>
                <TabsTrigger value="agent">Agent</TabsTrigger>
              </TabsList>
            </Tabs>
            {mode === "chat" && (
              <>
                <Select
                  value={provider}
                  onValueChange={(v) => {
                    setProvider(v);
                    const p = providers.find((x) => x.name === v);
                    if (p) setModel(p.default_model);
                  }}
                >
                  <SelectTrigger className="w-36 h-9 text-xs">
                    <SelectValue placeholder="Provider" />
                  </SelectTrigger>
                  <SelectContent>
                    {providers.map((p) => (
                      <SelectItem key={p.name} value={p.name}>
                        {p.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <input
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                  className="h-9 w-44 rounded-xl border border-white/10 bg-white/5 px-3 py-1 text-xs font-mono text-[var(--color-ink)] placeholder:text-[var(--color-ink-faint)] focus:outline-none focus:ring-2 focus:ring-[var(--color-lime)]/25"
                  placeholder="model"
                />
                {activeProvider && (
                  <Badge tone="lime" className="font-mono">
                    {activeProvider.name}
                  </Badge>
                )}
              </>
            )}
            {mode === "agent" && (
              <Badge tone="violet">
                <Sparkles className="h-3 w-3" />
                Orchestrator pipeline
              </Badge>
            )}
          </div>

          {/* Role picker — agent mode */}
          <AnimatePresence>
            {mode === "agent" && (
              <motion.div
                initial={{ opacity: 0, height: 0 }}
                animate={{ opacity: 1, height: "auto" }}
                exit={{ opacity: 0, height: 0 }}
                transition={springs.gentle}
                className="overflow-hidden"
              >
                <div className="flex items-center gap-2 mt-4 flex-wrap">
                  <span className="text-xs font-mono uppercase tracking-[0.22em] text-[var(--color-ink-faint)] mr-1">
                    Delegate to
                  </span>
                  {ROLES.map(({ role: r, abbr }) => (
                    <button
                      key={r}
                      onClick={() => setRole(r)}
                      data-cursor="hover"
                      className={cn(
                        "relative inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-all duration-200",
                        role === r
                          ? "scale-105"
                          : "border-white/10 bg-white/5 text-[var(--color-ink-muted)] hover:bg-white/10"
                      )}
                      style={
                        role === r
                          ? {
                              borderColor: roleAccents[r].border,
                              background: roleAccents[r].bg,
                              color: roleAccents[r].color,
                              boxShadow: `0 0 18px -6px ${roleAccents[r].color}`,
                            }
                          : undefined
                      }
                    >
                      <span
                        className="h-1.5 w-1.5 rounded-full"
                        style={{ background: roleAccents[r].color }}
                      />
                      {abbr}
                    </button>
                  ))}
                </div>
              </motion.div>
            )}
          </AnimatePresence>
        </div>
      </header>

      {/* Messages scroll area */}
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
              accent={roleAccent.color}
            />
          )}
        </div>
      </div>

      {/* Live activity panel — BELOW chat, ABOVE input. Shows real-time
       * agent activity with provider badges [LOCAL]/[CLOUD], token costs,
       * and a stopwatch timer. Claude Code-style. */}
      {streaming && (() => {
        const lastBubble = bubbles[bubbles.length - 1];
        const activityNodes = lastBubble?.activity ? treeAsArray(lastBubble.activity) : [];
        if (activityNodes.length === 0) return null;
        // Aggregate token costs by provider
        let localTokens = 0, cloudTokens = 0;
        for (const n of activityNodes) {
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
              <ActivityTree nodes={activityNodes} compact defaultExpanded={false} />
            </div>
          </div>
        );
      })()}

      {/* Input capsule */}
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
                    type="button"
                    onClick={() => setAttachments((prev) => prev.filter((x) => x.id !== f.id))}
                    disabled={streaming}
                    aria-label={`Remove ${f.filename}`}
                    className="text-[var(--color-ink-faint)] hover:text-[var(--color-coral)] disabled:opacity-30 transition-colors"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </span>
              ))}
            </div>
          )}
          <div className="glass-panel rounded-2xl p-2 flex gap-2 items-end focus-within:border-[var(--color-lime)]/30">
            <FileAttach
              disabled={streaming}
              onAdd={(f) => setAttachments((prev) => [...prev, f])}
              title={mode === "agent" ? "Attach file" : "Attach file (used when switching to Agent mode)"}
            />
            <Textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={onKeyDown}
              placeholder={
                mode === "agent"
                  ? `Describe a task for ${roleAccent.label}…`
                  : "Send a message…  (Enter = send, Shift+Enter = newline)"
              }
              className="flex-1 min-h-[44px] max-h-48 border-0 bg-transparent focus:ring-0 px-3"
            />
            <VoiceInput
              disabled={streaming}
              onTranscript={(text) => {
                setInput((prev) => {
                  if (!prev) return text.trimStart();
                  return prev.endsWith(" ") || prev.endsWith("\n")
                    ? prev + text.trimStart()
                    : prev + " " + text.trimStart();
                });
              }}
            />
            <AnimatePresence mode="wait" initial={false}>
              {streaming ? (
                <motion.div
                  key="stop"
                  initial={{ scale: 0.6, opacity: 0 }}
                  animate={{ scale: 1, opacity: 1 }}
                  exit={{ scale: 0.6, opacity: 0 }}
                  transition={springs.snappy}
                >
                  <Button variant="destructive" onClick={abort} size="icon">
                    <Square className="h-5 w-5" />
                  </Button>
                </motion.div>
              ) : (
                <motion.div
                  key="send"
                  initial={{ scale: 0.6, opacity: 0 }}
                  animate={{ scale: 1, opacity: 1 }}
                  exit={{ scale: 0.6, opacity: 0 }}
                  transition={springs.snappy}
                >
                  <Button
                    variant="lime"
                    onClick={() => send()}
                    disabled={!input.trim()}
                    size="icon"
                  >
                    <Send className="h-5 w-5" />
                  </Button>
                </motion.div>
              )}
            </AnimatePresence>
          </div>
          {providers.length === 0 && mode === "chat" && (
            <p className="mt-3 text-xs text-[var(--color-amber)] flex items-center gap-1.5">
              <AlertCircle className="h-3.5 w-3.5" />
              No providers available. Enable one in{" "}
              <Link
                to="/providers"
                className="underline underline-offset-2 hover:text-[var(--color-lime)]"
              >
                Providers
              </Link>{" "}
              and restart.
            </p>
          )}
        </div>
      </footer>
    </div>
  );
}

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
                {/* Live activity tree — replaces ThinkingDots while the agent
                 * runs in agent mode. Stays visible above the final answer
                 * once streaming completes (collapsible). */}
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
  accent,
}: {
  mode: "chat" | "agent";
  hasProviders: boolean;
  onPick: (text: string) => void;
  accent: string;
}) {
  return (
    <motion.div
      initial="hidden"
      animate="visible"
      variants={staggerContainer(0.1, 0.07)}
      className="text-center py-20"
    >
      <motion.div
        variants={staggerItem}
        className="inline-grid place-items-center h-20 w-20 rounded-3xl glass-panel mb-6"
        style={{ boxShadow: `0 0 50px -10px ${accent}` }}
      >
        <Sparkles className="h-9 w-9" style={{ color: accent }} />
      </motion.div>
      <motion.h2
        variants={staggerItem}
        className="text-3xl font-semibold tracking-tight mb-2"
        style={{ fontFamily: "var(--font-display)" }}
      >
        {mode === "agent" ? "What should we ship?" : "Start the conversation"}
      </motion.h2>
      <motion.p
        variants={staggerItem}
        className="text-sm text-[var(--color-ink-muted)] max-w-md mx-auto mb-8"
      >
        {mode === "agent"
          ? "Describe a task. The orchestrator will decompose, route, and execute it across the agent roster."
          : "Pick a provider, type a message. Switch to Agent mode for tools + orchestration."}
      </motion.p>
      {mode === "agent" && (
        <motion.div
          variants={staggerItem}
          className="grid sm:grid-cols-3 gap-3 max-w-2xl mx-auto"
        >
          {SUGGESTIONS.map((s, i) => (
            <motion.button
              key={i}
              variants={staggerItem}
              whileHover={{ y: -4, scale: 1.02 }}
              transition={springs.snappy}
              onClick={() => onPick(s.body)}
              data-cursor="hover"
              className="glass-panel rounded-2xl p-4 text-left hover:border-[var(--color-lime)]/30 transition-colors group"
            >
              <div
                className="h-8 w-8 grid place-items-center rounded-lg glass mb-3"
                style={{ color: "var(--color-lime)" }}
              >
                {s.icon}
              </div>
              <div className="text-sm font-medium text-[var(--color-ink)] mb-1 flex items-center gap-1">
                {s.title}
                <ArrowUpRight className="h-3 w-3 opacity-0 group-hover:opacity-100 transition-opacity" />
              </div>
              <div className="text-xs text-[var(--color-ink-muted)] line-clamp-2">
                {s.body}
              </div>
            </motion.button>
          ))}
        </motion.div>
      )}
      {!hasProviders && mode === "chat" && (
        <motion.div
          variants={staggerItem}
          className="mt-6 inline-flex items-center gap-1.5 text-xs text-[var(--color-amber)]"
        >
          <AlertCircle className="h-3.5 w-3.5" />
          No providers available — enable one in{" "}
          <Link to="/providers" className="underline">
            Providers
          </Link>
          .
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
