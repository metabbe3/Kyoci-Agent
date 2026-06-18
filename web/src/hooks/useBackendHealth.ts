/**
 * Shared backend liveness hook.
 *
 * `/health` is a low-frequency probe (not cacheable data), so it stays on a
 * plain `setInterval` poll rather than TanStack Query. To avoid every
 * component (Sidebar, Overview) opening its own interval, the result is backed
 * by a module-level store with a single ref-counted poller: the first mount
 * starts polling, the last unmount stops it, and every subscriber re-renders
 * on state change.
 */

import { useSyncExternalStore } from "react";
import { health } from "@/lib/api";
import { ApiError, ApiErrorKind } from "@/lib/api/errors";

export type BackendState = "checking" | "online" | "offline";

/** Poll cadence (ms). Tuned for "is the Go server up" — not latency-sensitive. */
const POLL_MS = 10_000;

// ── Shared ref-counted store ─────────────────────────────────────────────

type Listener = () => void;

let state: BackendState = "checking";
let listeners = new Set<Listener>();
let poller: ReturnType<typeof setInterval> | null = null;
let activeSubscribers = 0;
/** In-flight AbortController so an unmount cancels the pending probe. */
let inflight: AbortController | null = null;

function emit() {
  for (const l of listeners) l();
}

function setState(next: BackendState) {
  if (next === state) return;
  state = next;
  emit();
}

async function probe() {
  // Cancel any still-pending probe so we never have two racing.
  inflight?.abort();
  const ac = new AbortController();
  inflight = ac;
  try {
    await health();
    if (!ac.signal.aborted) setState("online");
  } catch (e) {
    if (ac.signal.aborted) return; // superseded or unmounted
    // Aborted-by-user probes don't flip state; everything else is offline.
    if (e instanceof ApiError && e.kind === ApiErrorKind.Aborted) return;
    setState("offline");
  } finally {
    if (inflight === ac) inflight = null;
  }
}

function start() {
  activeSubscribers += 1;
  if (activeSubscribers === 1) {
    probe();
    poller = setInterval(probe, POLL_MS);
  }
}

function stop() {
  activeSubscribers = Math.max(0, activeSubscribers - 1);
  if (activeSubscribers === 0) {
    if (poller !== null) {
      clearInterval(poller);
      poller = null;
    }
    inflight?.abort();
    inflight = null;
  }
}

function subscribe(l: Listener): () => void {
  listeners.add(l);
  start();
  return () => {
    listeners.delete(l);
    stop();
  };
}

function getSnapshot(): BackendState {
  return state;
}

/**
 * Subscribe to shared backend online/offline state. First subscriber starts a
 * 10s poll; last subscriber stops it. Every component sees the same value.
 */
export function useBackendHealth(): BackendState {
  // useSyncExternalStore guarantees tear-free reads across concurrent renders.
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}

/**
 * Trigger an immediate health re-probe (e.g. after the user clicks "retry").
 * Returns the AbortController so the caller can cancel if needed.
 */
export function probeBackendNow(): AbortController {
  void probe();
  return inflight ?? new AbortController();
}
