/**
 * TanStack Query wrappers around the dashboard REST endpoints.
 *
 * All data endpoints use a shared 30s `staleTime` so the three panels that read
 * providers (Overview, Providers, Sidebar/Chat) dedupe to a single network
 * request within that window. Network failures are retried by the client's
 * backoff; aborted requests are NOT retried (user intent).
 *
 * Liveness (health) and the long-poll status endpoint stay on plain
 * `useEffect`+`setInterval` hooks (see `useBackendHealth` / `useStatus`) — they
 * are low-frequency probes, not cacheable data fetches.
 */

import { useQuery, useQueryClient, type UseQueryOptions } from "@tanstack/react-query";
import { useCallback } from "react";
import { apiClient } from "@/lib/api/client";
import { ApiError, ApiErrorKind } from "@/lib/api/errors";
import { queryKeys } from "./query-keys";
import type {
  HardwareSpecs,
  ModelRow,
  ProviderConfigDTO,
  ProviderSummary,
  RecommendResult,
  SkillInfo,
} from "@/lib/types";

/** Shared defaults for read-only dashboard data. */
const DATA_OPTIONS = {
  staleTime: 30_000,
  gcTime: 5 * 60_000,
  refetchOnWindowFocus: false,
} as const;

/**
 * Decide whether TanStack should retry a failed query. We retry on transient
 * ApiError kinds (upstream/server/timeout/unreachable) but never on aborts
 * (the caller cancelled) or 4xx (the request shape is wrong — retrying won't
 * help).
 */
function shouldRetry(failureCount: number, error: unknown): boolean {
  if (failureCount >= 2) return false;
  if (error instanceof ApiError) {
    switch (error.kind) {
      case ApiErrorKind.Aborted:
      case ApiErrorKind.BadRequest:
      case ApiErrorKind.NotFound:
      case ApiErrorKind.Conflict:
        return false;
      default:
        return true;
    }
  }
  return true;
}

type DataQueryOptions<T> = Omit<UseQueryOptions<T>, "queryKey" | "queryFn">;

/** List of providers with availability flags (`/api/dashboard/providers`). */
export function useProviders(options?: DataQueryOptions<{ providers: ProviderSummary[] }>) {
  return useQuery({
    queryKey: queryKeys.providers,
    queryFn: ({ signal }) => apiClient.providers({ signal }),
    ...DATA_OPTIONS,
    retry: shouldRetry,
    ...options,
  });
}

/** Full model catalog (`/api/dashboard/models`). */
export function useModels(options?: DataQueryOptions<{ models: ModelRow[] }>) {
  return useQuery({
    queryKey: queryKeys.models,
    queryFn: ({ signal }) => apiClient.models({ signal }),
    ...DATA_OPTIONS,
    retry: shouldRetry,
    ...options,
  });
}

/** Per-provider editable config (`/api/dashboard/config`). */
export function useConfig(
  options?: DataQueryOptions<{ providers: Record<string, ProviderConfigDTO> }>
) {
  return useQuery({
    queryKey: queryKeys.config,
    queryFn: ({ signal }) => apiClient.getConfig({ signal }),
    ...DATA_OPTIONS,
    retry: shouldRetry,
    ...options,
  });
}

/** Auto-detected host specs (`/api/dashboard/hardware`). */
export function useHardware(options?: DataQueryOptions<HardwareSpecs>) {
  return useQuery({
    queryKey: queryKeys.hardware,
    queryFn: ({ signal }) => apiClient.hardware({ signal }),
    ...DATA_OPTIONS,
    retry: shouldRetry,
    ...options,
  });
}

/** Hardware-fit recommendation bundle (`/api/dashboard/recommendations`). */
export function useRecommendations(options?: DataQueryOptions<RecommendResult>) {
  return useQuery({
    queryKey: queryKeys.recommendations,
    queryFn: ({ signal }) => apiClient.recommendations({ signal }),
    ...DATA_OPTIONS,
    retry: shouldRetry,
    ...options,
  });
}

/** Zero-AI skill descriptors (`/api/dashboard/skills`). */
export function useSkills(options?: DataQueryOptions<{ skills: SkillInfo[] }>) {
  return useQuery({
    queryKey: queryKeys.skills,
    queryFn: ({ signal }) => apiClient.skills({ signal }),
    ...DATA_OPTIONS,
    retry: shouldRetry,
    ...options,
  });
}

/** All cacheable top-level query keys (excludes the per-provider helper). */
const ALL_DATA_KEYS = [
  queryKeys.providers,
  queryKeys.models,
  queryKeys.config,
  queryKeys.hardware,
  queryKeys.recommendations,
  queryKeys.skills,
] as const;

/**
 * Invalidate the dashboard data cache after a mutation (e.g. PUT config). Call
 * with no args to refetch everything, or specific keys for a targeted refresh.
 */
export function useInvalidateDashboard() {
  const qc = useQueryClient();
  return useCallback(
    (...keys: readonly (readonly unknown[])[]) => {
      const targets = keys.length ? keys : ALL_DATA_KEYS;
      return Promise.all(
        targets.map((k) => qc.invalidateQueries({ queryKey: k as readonly unknown[] }))
      );
    },
    [qc]
  );
}
