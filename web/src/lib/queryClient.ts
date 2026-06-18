/**
 * App-wide TanStack Query client.
 *
 * Defaults favor a dashboard UX: no automatic refetch on window focus (the
 * panels are user-initiated), a 30s default staleTime (overridden per-hook in
 * `hooks/useDashboardData.ts`), and retry only on transient failures (the
 * per-hook `retry` in `useDashboardData` classifies ApiError kinds).
 */

import { QueryClient } from "@tanstack/react-query";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 5 * 60_000,
      refetchOnWindowFocus: false,
      retry: 1,
    },
    mutations: {
      retry: 0,
    },
  },
});
