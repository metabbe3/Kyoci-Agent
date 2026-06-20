/**
 * Typed fetch-based API client for the System Monitor Dashboard.
 *
 * All backend communication flows through this module so that base URL,
 * timeout, and error normalization live in exactly one place. Every public
 * function is fully typed against the interfaces in `../types/metrics`.
 */

import type { ProcessInfo, SystemSnapshot } from '../types/metrics';

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

/**
 * Base URL for the backend API.
 *
 * Resolution order (first non-empty wins):
 *   1. Vite env var:  import.meta.env.VITE_API_BASE_URL
 *   2. Global var:    window.__API_BASE_URL__  (useful for runtime overrides)
 *   3. Default:       relative "/api"  (assumes same-origin Vite proxy)
 */
export const API_BASE_URL: string =
  (typeof import.meta !== 'undefined' &&
    import.meta.env &&
    (import.meta.env.VITE_API_BASE_URL as string | undefined)) ||
  (typeof window !== 'undefined' &&
    (window as unknown as { __API_BASE_URL__?: string }).__API_BASE_URL__) ||
  '/api';

/** Default request timeout in milliseconds. */
const DEFAULT_TIMEOUT_MS = 15000;

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Sort key supported by GET /api/processes. */
export type ProcessSortKey = 'cpu' | 'memory' | 'pid' | 'name';

/** Options accepted by {@link getProcesses}. */
export interface GetProcessesOptions {
  /** Field to sort by; defaults to 'cpu'. */
  sort?: ProcessSortKey;
  /** Maximum number of processes to return. */
  limit?: number;
}

/** Normalized API error carrying HTTP status and optional server payload. */
export class ApiError extends Error {
  readonly status: number;
  readonly payload: unknown;
  readonly endpoint: string;

  constructor(
    message: string,
    {
      status = 0,
      payload = null,
      endpoint = '',
    }: { status?: number; payload?: unknown; endpoint?: string } = {},
  ) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.payload = payload;
    this.endpoint = endpoint;
  }

  /** True when the failure was a network/timeout issue (no server response). */
  get isNetworkError(): boolean {
    return this.status === 0;
  }
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

/**
 * Build a full URL from a path and an optional query object.
 * Leading slash on `path` is optional; empty/null values are skipped.
 */
function buildUrl(path: string, query?: Record<string, unknown> | null): string {
  const base = API_BASE_URL.replace(/\/+$/, '');
  const cleanPath = path.startsWith('/') ? path : `/${path}`;
  let url = `${base}${cleanPath}`;

  if (query && Object.keys(query).length > 0) {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== null && value !== '') {
        params.append(key, String(value));
      }
    }
    const qs = params.toString();
    if (qs) url += `?${qs}`;
  }
  return url;
}

/**
 * Convert a raw `fetch` failure or non-2xx Response into an ApiError.
 */
async function toApiError(
  err: Response | Error | unknown,
  endpoint: string,
): Promise<ApiError> {
  // A thrown Error (network/abort) rather than a Response.
  if (!(err instanceof Response) && !(err && typeof err === 'object' && 'status' in err)) {
    const cause = err instanceof Error ? err : new Error(String(err));
    return new ApiError(cause.message || 'Network request failed', {
      status: 0,
      endpoint,
    });
  }

  const response = err as Response;
  let payload: unknown = null;
  let message = `Request failed with status ${response.status}`;
  try {
    payload = await response.json();
    const p = payload as { message?: string; error?: string } | null;
    message = (p && (p.message || p.error)) || message;
  } catch {
    // Response body was not JSON; keep the default message.
  }
  return new ApiError(message, {
    status: response.status,
    payload,
    endpoint,
  });
}

/**
 * Core request helper: perform a JSON request with timeout + error handling.
 */
async function request<T>(
  path: string,
  opts: {
    method?: string;
    query?: Record<string, unknown> | null;
    body?: unknown;
    timeoutMs?: number;
    headers?: Record<string, string>;
  } = {},
): Promise<T> {
  const {
    method = 'GET',
    query = null,
    body = null,
    timeoutMs = DEFAULT_TIMEOUT_MS,
    headers: extraHeaders = {},
  } = opts;

  const url = buildUrl(path, query);

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

  const headers: Record<string, string> = { Accept: 'application/json', ...extraHeaders };
  const init: RequestInit = { method, headers, signal: controller.signal };
  if (body !== null) {
    headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }

  let response: Response;
  try {
    response = await fetch(url, init);
  } catch (networkErr) {
    clearTimeout(timeoutId);
    throw await toApiError(networkErr, url);
  }
  clearTimeout(timeoutId);

  if (!response.ok) {
    throw await toApiError(response, url);
  }

  // 204 No Content or empty body → return null.
  if (response.status === 204) return null as T;
  const text = await response.text();
  return (text ? JSON.parse(text) : null) as T;
}

// ---------------------------------------------------------------------------
// Public API functions
// ---------------------------------------------------------------------------

/**
 * Fetch the current system snapshot (CPU, memory, disk, network, uptime).
 *
 * Corresponds to `GET /api/metrics`.
 */
export async function getMetrics(): Promise<SystemSnapshot> {
  return request<SystemSnapshot>('/metrics', { method: 'GET' });
}

/**
 * Fetch the list of running processes, optionally sorted and limited.
 *
 * Corresponds to `GET /api/processes?sort=<key>&limit=<n>`.
 *
 * @param sort   - Field to sort by; defaults to 'cpu' (descending).
 * @param limit  - Maximum number of rows to return.
 */
export async function getProcesses(
  sort?: ProcessSortKey,
  limit?: number,
): Promise<ProcessInfo[]> {
  const query: Record<string, unknown> = {};
  if (sort !== undefined) query.sort = sort;
  if (limit !== undefined) query.limit = limit;
  return request<ProcessInfo[]>('/processes', { method: 'GET', query });
}

// ---------------------------------------------------------------------------
// Convenience default export
// ---------------------------------------------------------------------------

const apiClient = {
  API_BASE_URL,
  ApiError,
  getMetrics,
  getProcesses,
};

export default apiClient;
