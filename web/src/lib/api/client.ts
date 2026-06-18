/**
 * Typed, OOP API client for the Kyoci-Agent dashboard REST surface
 * (`/api/dashboard/*` and `/api/v1/status`).
 *
 * Responsibilities:
 *  - One typed method per endpoint (return types live in `lib/types.ts`).
 *  - Centralized error handling: every failure throws {@link ApiError} with a
 *    categorized `kind`, so UI code never parses HTTP strings.
 *  - `AbortController` on every method (passed signal + an internal timeout
 *    deadline via {@link ApiClientOptions.timeoutMs}).
 *  - A pluggable {@link ApiLogger} hook for dev console / future telemetry.
 *  - Opt-in retry with exponential backoff for transient failures.
 *
 * The SSE streaming endpoint (`/api/dashboard/chat`) is NOT here — streaming
 * needs an async iterator and lives in `lib/api/sse.ts` (`chatStream`).
 */

import {
  ApiError,
  ApiErrorKind,
  kindFromNetworkError,
  kindFromStatus,
} from "./errors";
import type {
  HardwareSpecs,
  ModelRow,
  ProviderConfigDTO,
  ProviderSummary,
  RecommendResult,
  SkillInfo,
  UploadedFile,
} from "../types";

/** Same-origin in prod (Go embeds the bundle); Vite proxies `/api` + `/health` in dev. */
const DEFAULT_BASE = "";

/**
 * Pluggable logger. Receives every request (on success and failure) so a dev
 * shim can mirror to console and a prod shim can forward to telemetry. The
 * default no-op logger is free in hot paths.
 */
export interface ApiLogger {
  log(event: ApiLogEvent): void;
}

export type ApiLogEvent =
  | { kind: "request"; method: string; path: string; attempt: number }
  | { kind: "response"; method: string; path: string; status: number; ms: number; attempt: number }
  | { kind: "error"; method: string; path: string; error: ApiError; ms: number; attempt: number };

const noopLogger: ApiLogger = { log: () => {} };

/** Options shared by every request, overridable per-call. */
export interface RequestOptions {
  /** Caller-owned signal (e.g. from a useEffect cleanup or AbortController). */
  signal?: AbortSignal;
  /** Per-request timeout in ms. `0` = no internal deadline. */
  timeoutMs?: number;
  /** Override the client-level retry policy for this one call. */
  retries?: number;
}

export interface ApiClientOptions {
  /** Origin/base path prefix; defaults to same-origin. */
  base?: string;
  /** Default internal deadline for every request in ms. `0` disables. */
  timeoutMs?: number;
  /** Pluggable logger; defaults to no-op. */
  logger?: ApiLogger;
  /** Default number of retries for transient failures (default 0). */
  retries?: number;
  /** Base backoff in ms for the first retry (default 300). */
  backoffMs?: number;
}

/**
 * A categorized, retrying HTTP client. Construct once (see the exported
 * `apiClient` singleton) and reuse across panels; TanStack Query wraps it for
 * caching/dedup in `hooks/`.
 *
 * @example
 * const { data } = useQuery({ queryKey: ["providers"], queryFn: ({ signal }) => apiClient.providers({ signal }) });
 */
export class ApiClient {
  private readonly base: string;
  private readonly defaultTimeoutMs: number;
  private readonly logger: ApiLogger;
  private readonly defaultRetries: number;
  private readonly backoffMs: number;

  constructor(opts: ApiClientOptions = {}) {
    this.base = opts.base ?? DEFAULT_BASE;
    this.defaultTimeoutMs = opts.timeoutMs ?? 0;
    this.logger = opts.logger ?? noopLogger;
    this.defaultRetries = opts.retries ?? 0;
    this.backoffMs = opts.backoffMs ?? 300;
  }

  // ── Endpoint surface ────────────────────────────────────────────────────

  /** GET /api/dashboard/providers */
  providers(opts: RequestOptions = {}): Promise<{ providers: ProviderSummary[] }> {
    return this.getJSON("/api/dashboard/providers", opts);
  }

  /** GET /api/dashboard/models */
  models(opts: RequestOptions = {}): Promise<{ models: ModelRow[] }> {
    return this.getJSON("/api/dashboard/models", opts);
  }

  /** GET /api/dashboard/config */
  getConfig(opts: RequestOptions = {}): Promise<{ providers: Record<string, ProviderConfigDTO> }> {
    return this.getJSON("/api/dashboard/config", opts);
  }

  /** PUT /api/dashboard/config */
  putConfig(providers: Record<string, ProviderConfigDTO>, opts: RequestOptions = {}): Promise<{ ok: boolean; message: string }> {
    return this.putJSON("/api/dashboard/config", { providers }, opts);
  }

  /** POST /api/dashboard/test-connection */
  testConnection(provider: string, opts: RequestOptions = {}): Promise<{ available: boolean; error: string }> {
    return this.postJSON("/api/dashboard/test-connection", { provider }, opts);
  }

  /** GET /api/dashboard/hardware */
  hardware(opts: RequestOptions = {}): Promise<HardwareSpecs> {
    return this.getJSON("/api/dashboard/hardware", opts);
  }

  /** GET /api/dashboard/recommendations */
  recommendations(opts: RequestOptions = {}): Promise<RecommendResult> {
    return this.getJSON("/api/dashboard/recommendations", opts);
  }

  /** GET /api/dashboard/skills */
  skills(opts: RequestOptions = {}): Promise<{ skills: SkillInfo[] }> {
    return this.getJSON("/api/dashboard/skills", opts);
  }

  /** GET /api/v1/status (shape is backend-defined; kept as unknown here). */
  status(opts: RequestOptions = {}): Promise<unknown> {
    return this.getJSON("/api/v1/status", opts);
  }

  /** POST /api/dashboard/upload (multipart). */
  uploadFile(file: File, opts: RequestOptions = {}): Promise<UploadedFile> {
    const form = new FormData();
    form.append("file", file);
    return this.request("POST", "/api/dashboard/upload", { body: form, ...opts }).then((r) =>
      r.json() as Promise<UploadedFile>
    );
  }

  // ── Core HTTP plumbing ──────────────────────────────────────────────────

  /** GET with JSON body parse. */
  protected getJSON<T>(path: string, opts: RequestOptions = {}): Promise<T> {
    return this.request("GET", path, opts).then((r) => r.json() as Promise<T>);
  }

  /** POST JSON with JSON body parse. */
  protected postJSON<T>(path: string, body: unknown, opts: RequestOptions = {}): Promise<T> {
    return this.request("POST", path, { body, json: true, ...opts }).then((r) => r.json() as Promise<T>);
  }

  /** PUT JSON with JSON body parse. */
  protected putJSON<T>(path: string, body: unknown, opts: RequestOptions = {}): Promise<T> {
    return this.request("PUT", path, { body, json: true, ...opts }).then((r) => r.json() as Promise<T>);
  }

  /**
   * Perform a single (retrying) request. Merges the caller's signal with an
   * internal timeout controller, classifies all failures into {@link ApiError},
   * and retries transient kinds (upstream/server/timeout) when enabled.
   */
  protected async request(
    method: string,
    path: string,
    req: { body?: unknown; json?: boolean } & RequestOptions
  ): Promise<Response> {
    const url = `${this.base}${path}`;
    const maxAttempts = 1 + (req.retries ?? this.defaultRetries);
    let lastErr: ApiError | null = null;

    for (let attempt = 0; attempt < maxAttempts; attempt++) {
      if (attempt > 0) {
        // Exponential backoff with full jitter. Don't sleep after the final try.
        await sleep(Math.min(this.backoffMs * 2 ** (attempt - 1), 4000));
      }
      this.logger.log({ kind: "request", method, path, attempt });

      const { signal, cleanup } = this.resolveSignal(req);
      const started = performance.now();
      try {
        const headers: Record<string, string> = {};
        let body: BodyInit | undefined;
        if (req.json && req.body !== undefined) {
          headers["Content-Type"] = "application/json";
          body = JSON.stringify(req.body);
        } else if (req.body instanceof FormData) {
          body = req.body;
        }

        const r = await fetch(url, { method, headers, body, signal });
        const ms = Math.round(performance.now() - started);
        if (!r.ok) {
          const text = await safeText(r);
          const err = new ApiError(`${path}: ${r.status} ${text}`.trim(), {
            kind: kindFromStatus(r.status),
            status: r.status,
            body: text,
          });
          lastErr = err;
          this.logger.log({ kind: "error", method, path, error: err, ms, attempt });
          if (isRetryable(err.kind) && attempt < maxAttempts - 1) continue;
          throw err;
        }
        this.logger.log({ kind: "response", method, path, status: r.status, ms, attempt });
        return r;
      } catch (e) {
        cleanup();
        // Already an ApiError (from the !r.ok branch) — rethrow unless retryable.
        if (e instanceof ApiError) {
          if (isRetryable(e.kind) && attempt < maxAttempts - 1) {
            lastErr = e;
            continue;
          }
          throw e;
        }
        const ms = Math.round(performance.now() - started);
        const err = new ApiError(classifyNetwork(method, path, e), {
          kind: kindFromNetworkError(e),
          cause: e,
        });
        this.logger.log({ kind: "error", method, path, error: err, ms, attempt });
        // Aborts never retry.
        throw err;
      } finally {
        cleanup();
      }
    }
    // Unreachable in practice (loop throws on its last attempt), but keeps TS happy.
    throw lastErr ?? new ApiError(`${path}: exhausted retries`, { kind: ApiErrorKind.Unknown });
  }

  /**
   * Combine the caller's AbortSignal with an internal timeout deadline into one
   * signal, returning a cleanup that clears the deadline timer. Whichever
   * fires first wins; the resulting rejection is classified upstream.
   */
  private resolveSignal(req: RequestOptions): { signal: AbortSignal; cleanup: () => void } {
    const timeoutMs = req.timeoutMs ?? this.defaultTimeoutMs;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const cleanups: Array<() => void> = [];

    if (timeoutMs > 0) {
      timer = setTimeout(() => controller.abort(new DOMException("timeout", "TimeoutError")), timeoutMs);
      cleanups.push(() => clearTimeout(timer));
    }
    if (req.signal) {
      if (req.signal.aborted) controller.abort(req.signal.reason);
      else {
        const forward = () => controller.abort(req.signal!.reason);
        req.signal.addEventListener("abort", forward, { once: true });
        cleanups.push(() => req.signal!.removeEventListener("abort", forward));
      }
    }
    return { signal: controller.signal, cleanup: () => cleanups.forEach((fn) => fn()) };
  }
}

/** Transient kinds worth a retry: upstream drain, 5xx, or a timeout. */
function isRetryable(kind: ApiErrorKind): boolean {
  return (
    kind === ApiErrorKind.Upstream ||
    kind === ApiErrorKind.Server ||
    kind === ApiErrorKind.Timeout
  );
}

function classifyNetwork(method: string, path: string, e: unknown): string {
  const reason = e instanceof Error ? e.message : String(e);
  return `${method} ${path} failed: ${reason}`;
}

async function safeText(r: Response): Promise<string> {
  try {
    return await r.text();
  } catch {
    return "";
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((res) => setTimeout(res, ms));
}

/**
 * Shared dev logger that mirrors every request/response/error to the console.
 * Install via `new ApiClient({ logger: devConsoleLogger })`, or rely on the
 * singleton below which uses it when `import.meta.env.DEV`.
 */
export const devConsoleLogger: ApiLogger = {
  log(e) {
    switch (e.kind) {
      case "error":
        console.warn(`[api] ${e.method} ${e.path} → ${e.error.kind} (${e.ms}ms, attempt ${e.attempt + 1})`, e.error);
        break;
      case "response":
        console.debug(`[api] ${e.method} ${e.path} → ${e.status} (${e.ms}ms)`);
        break;
      default:
        break;
    }
  },
};

/**
 * App-wide singleton. Panels and TanStack Query hooks import this directly.
 * Dev builds get the console logger; prod is no-op.
 */
export const apiClient = new ApiClient({
  logger: import.meta.env.DEV ? devConsoleLogger : noopLogger,
});
