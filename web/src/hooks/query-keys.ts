/**
 * Centralized TanStack Query key factory.
 *
 * Co-locating keys here guarantees that every consumer of a given endpoint
 * shares one cache entry — e.g. Overview, Providers, and the Sidebar all call
 * `useProviders()` and now read from a single cached request instead of three
 * independent fetches.
 */

export const queryKeys = {
  providers: ["providers"] as const,
  models: ["models"] as const,
  config: ["config"] as const,
  hardware: ["hardware"] as const,
  recommendations: ["recommendations"] as const,
  skills: ["skills"] as const,
  /** A single provider's config — used after a targeted PUT to invalidate. */
  configProvider: (name: string) => ["config", name] as const,
} satisfies Record<string, unknown>;
