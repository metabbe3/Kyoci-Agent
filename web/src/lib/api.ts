import type {
  ProviderSummary,
  ModelRow,
  ProviderConfigDTO,
  HardwareSpecs,
  RecommendResult,
  UploadedFile,
  SkillInfo,
} from "./types";

const BASE = ""; // same origin in prod, proxied by Vite in dev

// BackendUnreachable is thrown when fetch rejects before any HTTP response —
// i.e., no server listening, connection refused, DNS failure, CORS preflight
// failure. HTTP 4xx/5xx responses are NOT this; those throw regular Error.
// Panels can `instanceof BackendUnreachable` to surface actionable guidance
// ("start the server") instead of Chrome's raw "Failed to fetch".
export class BackendUnreachable extends Error {
  constructor(public readonly path: string, cause: unknown) {
    super(`Cannot reach backend at ${path} — is the Go server running on :8080? (cause: ${cause instanceof Error ? cause.message : String(cause)})`);
    this.name = "BackendUnreachable";
  }
}

async function getJSON<T>(path: string): Promise<T> {
  let r: Response;
  try {
    r = await fetch(`${BASE}${path}`);
  } catch (e) {
    throw new BackendUnreachable(path, e);
  }
  if (!r.ok) throw new Error(`${path}: ${r.status} ${await r.text()}`);
  return r.json();
}

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  let r: Response;
  try {
    r = await fetch(`${BASE}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  } catch (e) {
    throw new BackendUnreachable(path, e);
  }
  if (!r.ok) throw new Error(`${path}: ${r.status} ${await r.text()}`);
  return r.json();
}

async function putJSON<T>(path: string, body: unknown): Promise<T> {
  let r: Response;
  try {
    r = await fetch(`${BASE}${path}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  } catch (e) {
    throw new BackendUnreachable(path, e);
  }
  if (!r.ok) throw new Error(`${path}: ${r.status} ${await r.text()}`);
  return r.json();
}

export const api = {
  providers: () => getJSON<{ providers: ProviderSummary[] }>("/api/dashboard/providers"),
  models: () => getJSON<{ models: ModelRow[] }>("/api/dashboard/models"),
  getConfig: () => getJSON<{ providers: Record<string, ProviderConfigDTO> }>("/api/dashboard/config"),
  putConfig: (providers: Record<string, ProviderConfigDTO>) =>
    putJSON<{ ok: boolean; message: string }>("/api/dashboard/config", { providers }),
  testConnection: (provider: string) =>
    postJSON<{ available: boolean; error: string }>("/api/dashboard/test-connection", { provider }),
  hardware: () => getJSON<HardwareSpecs>("/api/dashboard/hardware"),
  recommendations: () => getJSON<RecommendResult>("/api/dashboard/recommendations"),
  skills: () => getJSON<{ skills: SkillInfo[] }>("/api/dashboard/skills"),
  status: () => getJSON<unknown>("/api/v1/status"),
  uploadFile: async (file: File): Promise<UploadedFile> => {
    const form = new FormData();
    form.append("file", file);
    let r: Response;
    try {
      r = await fetch(`${BASE}/api/dashboard/upload`, {
        method: "POST",
        body: form,
      });
    } catch (e) {
      throw new BackendUnreachable("/api/dashboard/upload", e);
    }
    if (!r.ok) {
      const txt = await r.text();
      throw new Error(`/api/dashboard/upload: ${r.status} ${txt}`);
    }
    return r.json();
  },
};

// health probes /health and returns true on 200, false on any HTTP error, and
// throws BackendUnreachable on network failure. Used by the Sidebar poll.
export async function health(): Promise<boolean> {
  let r: Response;
  try {
    r = await fetch(`${BASE}/health`);
  } catch (e) {
    throw new BackendUnreachable("/health", e);
  }
  return r.ok;
}

// chatStream POSTs a ChatRequest and yields SSE chunks as they arrive. We
// can't use EventSource because it only does GET — fetch + ReadableStream
// reader is the standard pattern.
export async function* chatStream(
  req: import("./types").ChatRequest,
  signal?: AbortSignal
): AsyncIterableIterator<import("./types").SSEChunk> {
  let r: Response;
  try {
    r = await fetch(`${BASE}/api/dashboard/chat`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
      signal,
    });
  } catch (e) {
    if (e instanceof DOMException && e.name === "AbortError") throw e;
    throw new BackendUnreachable("/api/dashboard/chat", e);
  }
  if (!r.ok || !r.body) {
    throw new Error(`chat: ${r.status} ${await r.text()}`);
  }
  const reader = r.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) return;
    buffer += decoder.decode(value, { stream: true });
    let idx;
    while ((idx = buffer.indexOf("\n\n")) !== -1) {
      const block = buffer.slice(0, idx);
      buffer = buffer.slice(idx + 2);
      for (const line of block.split("\n")) {
        if (!line.startsWith("data: ")) continue;
        const data = line.slice(6);
        if (data === "[DONE]") return;
        try {
          yield JSON.parse(data) as import("./types").SSEChunk;
        } catch {
          // ignore malformed lines
        }
      }
    }
  }
}
