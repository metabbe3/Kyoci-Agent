/**
 * Centralized API error taxonomy for the Kyoci-Agent frontend.
 *
 * The {@link ApiErrorKind} enum mirrors the Go backend's `internal/apperr.Kind`
 * categories (see apperr/codes.go `Kind.String()`) plus the network-specific
 * states a browser fetch can land in (unreachable / aborted / timeout / parse).
 * The mapping is intentionally coarse: the frontend only needs enough category
 * to pick the right user-facing toast, not the full sentinel/Code machinery.
 *
 * HTTP-status → kind mapping is done by {@link kindFromStatus}; network failures
 * (fetch rejects before any response) are classified at the throw site.
 */

/**
 * Taxonomic category of an API failure. Mirrors `apperr.Kind` plus the
 * browser-only states (backend_unreachable, aborted, timeout, parse).
 *
 * @see internal/apperr/codes.go `Kind.String()`
 */
export enum ApiErrorKind {
  /** No server listening / connection refused / DNS / CORS preflight failed. */
  BackendUnreachable = "backend_unreachable",
  /** User (or component) aborted the request via AbortController. */
  Aborted = "aborted",
  /** Request exceeded its deadline (fetch timeout or 504). */
  Timeout = "timeout",
  /** 400 — malformed request / invalid input (apperr KindInvalid). */
  BadRequest = "bad_request",
  /** 404 — resource does not exist (apperr KindNotFound). */
  NotFound = "not_found",
  /** 409 — concurrent modification / duplicate (apperr KindConflict). */
  Conflict = "conflict",
  /** 429 — too many requests (apperr KindProviderExhausted adjacent). */
  RateLimited = "rate_limited",
  /** 503 — provider/circuit open / upstream drained (apperr KindUnavailable). */
  Upstream = "upstream",
  /** 500 / other 5xx — backend bug (apperr KindInternal/Unknown). */
  Server = "server",
  /** Response body could not be parsed as expected JSON. */
  Parse = "parse",
  /** Anything not covered above. */
  Unknown = "unknown",
}

/**
 * A categorized API error. Thrown by {@link ApiClient} for every non-2xx
 * response and every network/parse failure, so UI code can branch on
 * `e.kind` (or `instanceof ApiError`) instead of parsing HTTP strings.
 *
 * The legacy {@link BackendUnreachable} (in api.ts shim) is preserved as a
 * subclass so `instanceof BackendUnreachable` checks in panels keep working.
 */
export class ApiError extends Error {
  /** HTTP status code, or 0 for network/parse failures with no response. */
  readonly status: number;
  /** Raw response body text (best-effort, may be empty for streams). */
  readonly body: string;
  /** The categorized kind driving UX. */
  readonly kind: ApiErrorKind;
  /** Underlying cause (e.g. the original TypeError from fetch). */
  override readonly cause: unknown;

  constructor(
    message: string,
    opts: { kind: ApiErrorKind; status?: number; body?: string; cause?: unknown }
  ) {
    super(message);
    this.name = "ApiError";
    this.kind = opts.kind;
    this.status = opts.status ?? 0;
    this.body = opts.body ?? "";
    this.cause = opts.cause;
  }

  /** True when the failure was a user-initiated abort. */
  get isAborted(): boolean {
    return this.kind === ApiErrorKind.Aborted;
  }

  /** True when the backend could not be reached at all (no HTTP response). */
  get isUnreachable(): boolean {
    return this.kind === ApiErrorKind.BackendUnreachable;
  }
}

/**
 * Map an HTTP status code to an {@link ApiErrorKind}. Mirrors
 * `apperr.CodeToHTTP` in reverse (status → kind) for the categories the UI
 * actually distinguishes; unknown codes fall through to {@link ApiErrorKind.Server}.
 */
export function kindFromStatus(status: number): ApiErrorKind {
  switch (status) {
    case 400:
      return ApiErrorKind.BadRequest;
    case 404:
      return ApiErrorKind.NotFound;
    case 408:
      return ApiErrorKind.Timeout;
    case 409:
      return ApiErrorKind.Conflict;
    case 429:
      return ApiErrorKind.RateLimited;
    case 502:
    case 503:
    case 504:
      return ApiErrorKind.Upstream;
    default:
      return status >= 500
        ? ApiErrorKind.Server
        : status >= 400
          ? ApiErrorKind.BadRequest
          : ApiErrorKind.Unknown;
  }
}

/**
 * Classify a network-layer failure (fetch rejected before producing a
 * Response). AbortError → aborted; otherwise the server is unreachable.
 */
export function kindFromNetworkError(e: unknown): ApiErrorKind {
  if (e instanceof DOMException && e.name === "AbortError") {
    return ApiErrorKind.Aborted;
  }
  // Timeout manifests as an AbortError fired by our own deadline timer; fetch
  // itself only rejects with TypeError for genuine connectivity loss.
  if (e instanceof Error && /timeout/i.test(e.name + e.message)) {
    return ApiErrorKind.Timeout;
  }
  return ApiErrorKind.BackendUnreachable;
}
