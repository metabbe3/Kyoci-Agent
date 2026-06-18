/**
 * Centralized API-error → toast mapping.
 *
 * Instead of ad-hoc `toast.error(e.message)` scattered across panels, every
 * caught {@link ApiError} routes through {@link toastApiError}, which picks a
 * sonner variant and an actionable description from the error's `kind`.
 * User-initiated aborts are silent (no toast) because nothing went wrong.
 */

import { toast } from "sonner";
import { ApiError, ApiErrorKind } from "./errors";

export interface ToastCtx {
  /** Short label for the action that failed, e.g. "Save provider" or "Load models". */
  action?: string;
  /** Extra detail appended to the description (e.g. a provider name). */
  detail?: string;
}

interface ToastPlan {
  title: string;
  description: string;
  variant: "default" | "success" | "error" | "warning" | "info";
}

/**
 * Map an {@link ApiError} to a toast plan. Exported so callers can inspect the
 * plan without showing it (e.g. for tests).
 */
export function planApiErrorToast(e: ApiError, ctx: ToastCtx = {}): ToastPlan | null {
  const prefix = ctx.action ? `${ctx.action}: ` : "";
  switch (e.kind) {
    case ApiErrorKind.Aborted:
      // The user (or a cleanup) cancelled the request — nothing went wrong.
      return null;
    case ApiErrorKind.BackendUnreachable:
      return {
        title: `${prefix}Backend unreachable`,
        description: ctx.detail ?? "Start the Go server: `go run ./cmd/server`.",
        variant: "error",
      };
    case ApiErrorKind.Timeout:
      return {
        title: `${prefix}Request timed out`,
        description: ctx.detail ?? "The server took too long to respond. Try again.",
        variant: "warning",
      };
    case ApiErrorKind.BadRequest:
      return {
        title: `${prefix}Invalid request`,
        description: e.body || ctx.detail || "The server rejected the request.",
        variant: "warning",
      };
    case ApiErrorKind.NotFound:
      return {
        title: `${prefix}Not found`,
        description: ctx.detail ?? "The resource is gone or was never created.",
        variant: "warning",
      };
    case ApiErrorKind.Conflict:
      return {
        title: `${prefix}Conflict`,
        description: ctx.detail ?? "The resource changed on the server — refresh and retry.",
        variant: "warning",
      };
    case ApiErrorKind.RateLimited:
      return {
        title: `${prefix}Rate limited`,
        description: ctx.detail ?? "Too many requests — wait a moment and retry.",
        variant: "warning",
      };
    case ApiErrorKind.Upstream:
      return {
        title: `${prefix}Provider unavailable`,
        description: ctx.detail ?? (e.body || "An upstream provider is drained or circuit-open."),
        variant: "error",
      };
    case ApiErrorKind.Parse:
      return {
        title: `${prefix}Bad response`,
        description: ctx.detail ?? "The server returned data we couldn't parse.",
        variant: "warning",
      };
    case ApiErrorKind.Server:
    case ApiErrorKind.Unknown:
    default:
      return {
        title: `${prefix}Something went wrong`,
        description: e.body || e.message || "An unexpected error occurred.",
        variant: "error",
      };
  }
}

/**
 * Show a toast for an API failure. Silently no-ops on user aborts and on
 * non-{@link ApiError} errors (those are surfaced by the caller or the render
 * boundary). Returns the toast id (or undefined when nothing was shown).
 */
export function toastApiError(e: unknown, ctx: ToastCtx = {}): string | number | undefined {
  if (!(e instanceof ApiError)) {
    // Fallback for legacy Error throws: surface the message but don't crash.
    const message = e instanceof Error ? e.message : String(e);
    return toast.error(ctx.action ? `${ctx.action} failed` : "Request failed", {
      description: message,
    });
  }
  const plan = planApiErrorToast(e, ctx);
  if (!plan) return undefined;
  return toast[plan.variant === "default" ? "message" : plan.variant](plan.title, {
    description: plan.description,
  });
}
