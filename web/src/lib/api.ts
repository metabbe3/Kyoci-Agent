/**
 * Backward-compatibility shim.
 *
 * The real implementations now live in `lib/api/`:
 *   - `ApiClient` + singleton `apiClient` → `lib/api/client.ts`
 *   - `ApiError` / `ApiErrorKind`          → `lib/api/errors.ts`
 *   - `chatStream`                          → `lib/api/sse.ts`
 *
 * This module re-exports them and keeps the historical surface (`api`, `health`,
 * `BackendUnreachable`, `chatStream`) so existing panel imports keep working
 * during the gradual migration to TanStack Query + the new client class.
 *
 * Prefer importing from `@/lib/api/client`, `@/lib/api/errors`, `@/lib/api/sse`,
 * or the `@/hooks/*` wrappers in new code.
 */

import { ApiClient, apiClient } from "./api/client";
import { ApiError, ApiErrorKind } from "./api/errors";
export { ApiError, ApiErrorKind } from "./api/errors";
export { ApiClient, apiClient, devConsoleLogger } from "./api/client";
export type { ApiLogger, ApiLogEvent, ApiClientOptions, RequestOptions } from "./api/client";
export { chatStream } from "./api/sse";

/**
 * Legacy network-unreachable error. Now a thin subclass of {@link ApiError}
 * (kind = `backend_unreachable`) so existing `instanceof BackendUnreachable`
 * checks in panels keep working after the migration.
 */
export class BackendUnreachable extends ApiError {
  constructor(public readonly path: string, cause: unknown) {
    const reason = cause instanceof Error ? cause.message : String(cause);
    super(
      `Cannot reach backend at ${path} — is the Go server running on :8080? (cause: ${reason})`,
      { kind: ApiErrorKind.BackendUnreachable, cause }
    );
    this.name = "BackendUnreachable";
  }
}

/**
 * The historical `api` object. Delegates to the singleton {@link apiClient};
 * method shapes (e.g. `api.providers(signal?)`) are unchanged so panels that
 * have not yet migrated compile and behave identically.
 */
export const api = {
  providers: (signal?: AbortSignal) => apiClient.providers({ signal }),
  models: (signal?: AbortSignal) => apiClient.models({ signal }),
  getConfig: (signal?: AbortSignal) => apiClient.getConfig({ signal }),
  putConfig: (providers: Parameters<ApiClient["putConfig"]>[0], signal?: AbortSignal) =>
    apiClient.putConfig(providers, { signal }),
  testConnection: (provider: string, signal?: AbortSignal) =>
    apiClient.testConnection(provider, { signal }),
  hardware: (signal?: AbortSignal) => apiClient.hardware({ signal }),
  recommendations: (signal?: AbortSignal) => apiClient.recommendations({ signal }),
  skills: (signal?: AbortSignal) => apiClient.skills({ signal }),
  status: (signal?: AbortSignal) => apiClient.status({ signal }),
  uploadFile: (file: File, signal?: AbortSignal) => apiClient.uploadFile(file, { signal }),
};

/**
 * health probes `/health` and returns true on 200, false on any HTTP error,
 * and throws {@link BackendUnreachable} on network failure. Used by the
 * Sidebar / Overview poll hooks. Kept on its own poll (not TanStack Query)
 * because it is a low-frequency liveness check, not a data fetch.
 */
export async function health(): Promise<boolean> {
  let r: Response;
  try {
    r = await fetch("/health");
  } catch (e) {
    throw new BackendUnreachable("/health", e);
  }
  return r.ok;
}
