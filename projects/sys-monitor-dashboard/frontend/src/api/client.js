/**
 * Reusable fetch-based API client for the System Monitor Dashboard.
 *
 * All backend communication flows through this module so that base URL,
 * auth headers, and error normalization live in exactly one place.
 */

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

/**
 * Base URL for the backend API.
 *
 * Resolution order (first non-empty wins):
 *   1. Vite env var:  import.meta.env.VITE_API_BASE_URL
 *   2. Global var:    window.__API_BASE_URL__  (useful for runtime overrides)
 *   3. Default:       relative "/api/v1"  (assumes same-origin proxy)
 */
export const API_BASE_URL =
  (typeof import.meta !== 'undefined' &&
    import.meta.env &&
    import.meta.env.VITE_API_BASE_URL) ||
  (typeof window !== 'undefined' && window.__API_BASE_URL__) ||
  '/api/v1';

/** Default request timeout in milliseconds. */
const DEFAULT_TIMEOUT_MS = 15000;

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

/**
 * Normalized API error. Carries HTTP status, a human-readable message, and
 * (when available) the raw server payload so callers can render details.
 */
export class ApiError extends Error {
  constructor(message, { status = 0, payload = null, endpoint = '' } = {}) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.payload = payload;
    this.endpoint = endpoint;
  }

  /** True when the failure was a network/timeout issue (no server response). */
  get isNetworkError() {
    return this.status === 0;
  }
}

/**
 * Convert a raw `fetch` failure or non-2xx response into an ApiError.
 * @param {Response|Error} err - Either a Response object or a thrown Error.
 * @param {string} endpoint - The URL that was requested.
 * @returns {Promise<ApiError>}
 */
async function toApiError(err, endpoint) {
  // A thrown Error (network/abort) rather than a Response.
  if (!(err instanceof Response) && !(err && err.status)) {
    const cause = err instanceof Error ? err : new Error(String(err));
    return new ApiError(cause.message || 'Network request failed', {
      status: 0,
      endpoint,
    });
  }

  const response = err instanceof Response ? err : err;
  let payload = null;
  let message = `Request failed with status ${response.status}`;
  try {
    payload = await response.json();
    message = (payload && (payload.message || payload.error)) || message;
  } catch {
    // Response body was not JSON; keep the default message.
  }
  return new ApiError(message, {
    status: response.status,
    payload,
    endpoint,
  });
}

// ---------------------------------------------------------------------------
// Core request helper
// ---------------------------------------------------------------------------

/**
 * Perform a JSON GET/POST against the backend with timeout + error handling.
 *
 * @param {string} path     - Path appended to API_BASE_URL (leading "/" optional).
 * @param {object} [opts]
 * @param {string} [opts.method='GET']
 * @param {object} [opts.query]   - Query params, serialized via URLSearchParams.
 * @param {object} [opts.body]    - JSON-serializable request body.
 * @param {number} [opts.timeoutMs]
 * @param {object} [opts.headers] - Extra headers.
 * @returns {Promise<any>} Parsed JSON response.
 */
export async function request(path, opts = {}) {
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

  const headers = { Accept: 'application/json', ...extraHeaders };
  const init = { method, headers, signal: controller.signal };
  if (body !== null) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }

  let response;
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
  if (response.status === 204) return null;
  const text = await response.text();
  return text ? JSON.parse(text) : null;
}

/**
 * Build a full URL from a path and optional query object.
 * @param {string} path
 * @param {object|null} query
 * @returns {string}
 */
function buildUrl(path, query) {
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

// ---------------------------------------------------------------------------
// Public API functions
// ---------------------------------------------------------------------------

/**
 * Fetch a paginated, filterable list of log entries.
 *
 * @param {number} [page=1]       - 1-indexed page number.
 * @param {string} [level]        - Filter by log level: 'INFO' | 'WARN' | 'ERROR' (empty = all).
 * @param {string} [timeRange]    - Time window: e.g. '15m' | '1h' | '24h' | '7d' (empty = all time).
 * @returns {Promise<{items: Array, page: number, pageSize: number, total: number}>}
 */
export async function fetchLogs(page = 1, level = '', timeRange = '') {
  return request('/logs', {
    method: 'GET',
    query: {
      page,
      level: level || undefined,
      time_range: timeRange || undefined,
    },
  });
}

/**
 * Fetch aggregate statistics for the dashboard (counts by level, trends, etc.).
 * @returns {Promise<object>}
 */
export async function fetchStats() {
  return request('/stats', { method: 'GET' });
}

/**
 * Trigger an on-demand scan of the configured log directories.
 * @returns {Promise<object>} Scan result summary (e.g. { scanned, inserted, durationMs }).
 */
export async function triggerScan() {
  return request('/scan', { method: 'POST' });
}

/**
 * Fetch the AI-generated executive summary of recent system health.
 * @returns {Promise<{summary: string, generatedAt: string, windowHours: number}>}
 */
export async function fetchSummary() {
  return request('/summary', { method: 'GET' });
}

// ---------------------------------------------------------------------------
// Convenience default export
// ---------------------------------------------------------------------------

const apiClient = {
  request,
  fetchLogs,
  fetchStats,
  triggerScan,
  fetchSummary,
  API_BASE_URL,
};

export default apiClient;
