/**
 * Streaming chat orchestration extracted from `ChatPanel`.
 *
 * Owns the streaming lifecycle (the `streaming` flag + the AbortController) and
 * the `send` loop that consumes the SSE stream from `lib/api/sse`. The panel
 * supplies the message history and callbacks to mutate its bubbles, keeping
 * this hook free of JSX and re-usable.
 *
 * Malformed SSE lines are already surfaced as `{ error }` chunks by
 * `chatStream` (see `lib/api/sse.ts`); this hook renders them as an error
 * bubble, where the old code silently dropped them.
 */

import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { chatStream } from "@/lib/api/sse";
import { ApiError, ApiErrorKind } from "@/lib/api/errors";
import { toastApiError } from "@/lib/api/toast";
import { applyActivityEvent } from "@/components/ActivityTree";
import type { ChatMessage, SSEChunk, UploadedFile, TreeNode, ActivityEvent } from "@/lib/types";

/** One rendered chat turn. `error` marks a turn that ended in failure.
 * `activity` carries the live activity tree for agent-mode turns (empty
 * for chat-mode). */
export interface ChatTurn {
  role: "user" | "assistant";
  content: string;
  error?: boolean;
  /** Live activity tree rows keyed by task_id. Built incrementally from
   * activity events in the SSE stream. */
  activity?: Map<string, TreeNode>;
}

export interface SendParams {
  /** "chat" needs a provider/model; "agent" may attach files. */
  mode: "chat" | "agent";
  provider?: string;
  model?: string;
  /** Existing turns to seed the request history (not including the new one). */
  history: ChatMessage[];
  /** Files to attach in agent mode. */
  files?: UploadedFile[];
}

export interface UseChatStreamOptions {
  /** Append the user bubble + an empty assistant bubble before streaming. */
  onTurnStart: (user: ChatTurn) => void;
  /** Replace the in-flight assistant bubble (e.g. append content, mark error). */
  onUpdateLast: (update: (prev: ChatTurn) => ChatTurn) => void;
  /** Drop the trailing assistant bubble (used on backend-unreachable). */
  onDropLast: () => void;
}

export interface UseChatStreamResult {
  /** True while a response is streaming. */
  streaming: boolean;
  /** Send a message and stream the reply. Returns when the stream ends. */
  send: (text: string, params: SendParams) => Promise<void>;
  /** Abort the in-flight stream (user pressed stop). */
  abort: () => void;
}

/**
 * Drive a single chat stream. The hook holds the AbortController so the panel
 * doesn't manage refs for it; `onTurnStart`/`onUpdateLast`/`onDropLast` keep
 * bubble state in the component.
 */
export function useChatStream(opts: UseChatStreamOptions): UseChatStreamResult {
  const [streaming, setStreaming] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  // Reset the abort ref + flag if the component holding us ever unmounts mid-stream.
  useEffect(() => {
    return () => {
      abortRef.current?.abort();
      abortRef.current = null;
    };
  }, []);

  const send = useCallback(
    async (text: string, params: SendParams) => {
      const trimmed = text.trim();
      if (!trimmed) return;

      const userTurn: ChatTurn = { role: "user", content: trimmed };
      opts.onTurnStart(userTurn);
      setStreaming(true);

      const ac = new AbortController();
      abortRef.current = ac;

      try {
        const stream = chatStream(
          {
            mode: params.mode,
            provider: params.mode === "chat" ? params.provider : undefined,
            model: params.mode === "chat" && params.model ? params.model : undefined,
            messages: params.history,
            files: params.mode === "agent" && params.files && params.files.length > 0 ? params.files : undefined,
          },
          ac.signal
        );

        for await (const chunk of stream) {
          applyChunk(chunk, opts.onUpdateLast);
          if (chunk.error) break;
          if (chunk.done) break;
        }
      } catch (e) {
        handleStreamError(e, { onUpdateLast: opts.onUpdateLast, onDropLast: opts.onDropLast });
      } finally {
        setStreaming(false);
        abortRef.current = null;
      }
    },
    [opts]
  );

  const abort = useCallback(() => {
    abortRef.current?.abort();
    setStreaming(false);
  }, []);

  return { streaming, send, abort };
}

/** Fold a single SSE chunk into the in-flight assistant turn. */
function applyChunk(chunk: SSEChunk, onUpdateLast: (update: (prev: ChatTurn) => ChatTurn) => void) {
  if (chunk.error) {
    // Includes the malformed-line errors now surfaced by chatStream (sse.ts).
    onUpdateLast((prev) => ({
      ...prev,
      error: true,
      content: prev.content || chunk.error!,
    }));
    return;
  }
  if (chunk.activity) {
    // Route into the per-turn activity tree. We carry the Map on the turn
    // so it survives content updates and so the component can render the
    // tree above the streaming answer.
    onUpdateLast((prev) => {
      const evt: ActivityEvent = chunk.activity!;
      const tree = prev.activity ? new Map(prev.activity) : new Map<string, TreeNode>();
      const next = applyActivityEvent(tree, evt);
      return { ...prev, activity: next };
    });
    return;
  }
  if (chunk.content) {
    onUpdateLast((prev) => ({ ...prev, content: prev.content + chunk.content! }));
  }
}

/** Classify and surface a stream failure; never throws to the panel. */
function handleStreamError(
  e: unknown,
  out: { onUpdateLast: (u: (p: ChatTurn) => ChatTurn) => void; onDropLast: () => void }
) {
  // User aborts: no error surface, no toast.
  if (e instanceof DOMException && e.name === "AbortError") return;
  if (e instanceof ApiError && e.kind === ApiErrorKind.Aborted) return;

  if (e instanceof ApiError && e.isUnreachable) {
    // Mirror the legacy UX: drop the empty assistant bubble and toast guidance.
    out.onDropLast();
    toastApiError(e, { action: "Chat", detail: "Start the Go server: `go run ./cmd/server`." });
    return;
  }
  // Any other failure becomes an error bubble so the user sees what happened.
  const message = e instanceof Error ? e.message : String(e);
  out.onUpdateLast(() => ({ role: "assistant", content: `Error: ${message}`, error: true }));
}
