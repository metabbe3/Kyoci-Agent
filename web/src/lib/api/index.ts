/**
 * Barrel for the API layer.
 *
 *   import { apiClient, ApiError, ApiErrorKind, chatStream } from "@/lib/api/client";
 *
 * (The historical `api`/`health`/`BackendUnreachable` surface stays at
 * `@/lib/api` — the shim — for backward compatibility.)
 */

export * from "./errors";
export * from "./client";
export * from "./sse";
