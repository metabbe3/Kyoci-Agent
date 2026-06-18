/**
 * Server-Sent Events streaming for the chat endpoint
 * (`POST /api/dashboard/chat`).
 *
 * Browsers can't use `EventSource` here because the chat request is a POST
 * with a JSON body, so we parse the `text/event-stream` manually from a
 * `ReadableStream` reader.
 *
 * Parse errors are surfaced as an {@link SSEChunk} with `error` set — the old
 * implementation silently dropped malformed lines, which hid server bugs.
 */

import { ApiError, ApiErrorKind, kindFromNetworkError } from "./errors";
import type { ChatRequest, SSEChunk } from "../types";

/** Sentinel the server emits to end a stream cleanly. */
const DONE = "[DONE]";

/**
 * POST a {@link ChatRequest} and yield {@link SSEChunk}s as they arrive.
 *
 * Network failures (no server, aborted, timeout) throw {@link ApiError}; HTTP
 * non-2xx responses throw {@link ApiError} classified from the status. Malformed
 * SSE data lines are not thrown — they are yielded as an `error` chunk so the
 * UI can surface "stream was malformed" instead of silently stalling.
 *
 * @example
 * for await (const chunk of chatStream(req, controller.signal)) {
 *   if (chunk.error) { showError(chunk.error); break; }
 *   if (chunk.content) append(chunk.content);
 *   if (chunk.done) break;
 * }
 */
export async function* chatStream(
  req: ChatRequest,
  signal?: AbortSignal
): AsyncIterableIterator<SSEChunk> {
  let r: Response;
  try {
    r = await fetch("/api/dashboard/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
      signal,
    });
  } catch (e) {
    // User aborts propagate as the original DOMException so callers can detect
    // AbortError; everything else is categorized.
    if (e instanceof DOMException && e.name === "AbortError") throw e;
    throw new ApiError(`chat: ${causeMessage(e)}`, { kind: kindFromNetworkError(e), cause: e });
  }

  if (!r.ok || !r.body) {
    const text = await safeText(r);
    throw new ApiError(`chat: ${r.status} ${text}`.trim(), {
      kind: ApiErrorKind.Unknown,
      status: r.status,
      body: text,
    });
  }

  const reader = r.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) return;
      buffer += decoder.decode(value, { stream: true });

      let idx: number;
      while ((idx = buffer.indexOf("\n\n")) !== -1) {
        const block = buffer.slice(0, idx);
        buffer = buffer.slice(idx + 2);
        for (const line of block.split("\n")) {
          if (!line.startsWith("data: ")) continue;
          const data = line.slice(6);
          if (data === DONE) return;
          try {
            yield JSON.parse(data) as SSEChunk;
          } catch (e) {
            // Surface malformed lines instead of swallowing them. The UI shows
            // this as an error bubble; the server author can then fix the wire
            // format. We keep streaming (don't return) in case later lines parse.
            yield { error: `Malformed SSE data: ${causeMessage(e)}` };
          }
        }
      }
    }
  } finally {
    // Release the reader's lock on the underlying stream even on early break.
    reader.releaseLock();
  }
}

function causeMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

async function safeText(r: Response): Promise<string> {
  try {
    return await r.text();
  } catch {
    return "";
  }
}
