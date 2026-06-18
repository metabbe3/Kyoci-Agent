import { useEffect, useRef, useState } from "react";
import { applyActivityEvent } from "@/components/ActivityTree";
import type { ActivityEvent, TreeNode } from "@/lib/types";

/**
 * useActivityFeed — subscribe to the global `/api/dashboard/activity` SSE
 * stream and accumulate events into a tree.
 *
 * Used by the Live Activity panel. Multiplexes every event from every running
 * agent across the whole app. Events arrive as:
 *
 *   event: activity
 *   data: {"activity":{"type":"task_start","task_id":"step-1",...}}
 *
 * The hook parses, applies via the shared reducer, and exposes the current
 * tree as a top-level array of TreeNodes (children nested).
 *
 * Auto-reconnects on browser-managed EventSource close with exponential
 * backoff (1s → 2s → 4s → max 30s).
 */
export function useActivityFeed(): {
  tree: TreeNode[];
  raw: Map<string, TreeNode>;
  subscriberCount: number;
  connected: boolean;
} {
  const [raw, setRaw] = useState<Map<string, TreeNode>>(new Map());
  const [connected, setConnected] = useState(false);
  const [subscriberCount, setSubscriberCount] = useState(1); // self
  const backoffRef = useRef(1000);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    let stopped = false;

    const connect = () => {
      if (stopped) return;
      const es = new EventSource("/api/dashboard/activity");
      esRef.current = es;

      es.onopen = () => {
        if (stopped) return;
        setConnected(true);
        backoffRef.current = 1000; // reset on success
      };

      es.onerror = () => {
        setConnected(false);
        es.close();
        // Exponential backoff, capped at 30s.
        const delay = Math.min(backoffRef.current, 30_000);
        backoffRef.current = Math.min(backoffRef.current * 2, 30_000);
        window.setTimeout(connect, delay);
      };

      es.addEventListener("activity", (e: MessageEvent) => {
        if (stopped) return;
        try {
          const payload = JSON.parse(e.data);
          const evt: ActivityEvent = payload.activity;
          if (!evt || !evt.type || !evt.task_id) return;
          setRaw((prev) => {
            const next = new Map(prev);
            return applyActivityEvent(next, evt);
          });
          // Prune done+error rows older than 60s to keep the panel scannable.
          // We don't prune inside the reducer so the inline chat tree keeps
          // its final state; this is a panel-only concern.
          setRaw((prev) => pruneStale(prev));
        } catch {
          // Ignore malformed events; the wire format is debuggable via the
          // browser devtools network panel.
        }
      });
    };

    connect();
    return () => {
      stopped = true;
      esRef.current?.close();
      esRef.current = null;
    };
  }, []);

  // Compute the top-level array from the map. Memoize via useState/useEffect
  // pattern so the panel doesn't re-sort on every keystroke elsewhere.
  const tree = useMemoTree(raw);
  return { tree, raw, subscriberCount, connected };
}

/** Filter completed rows older than 60s. Running rows always survive. */
function pruneStale(tree: Map<string, TreeNode>): Map<string, TreeNode> {
  const cutoff = Date.now() - 60_000;
  let changed = false;
  const next = new Map<string, TreeNode>();
  for (const [id, node] of tree) {
    if (node.status === "running") {
      next.set(id, node);
    } else if (node.finishedAt && node.finishedAt >= cutoff) {
      next.set(id, node);
    } else {
      changed = true;
    }
  }
  return changed ? next : tree;
}

// Local memo helper — recomputes only when the map identity changes.
import { useMemo } from "react";
function useMemoTree(raw: Map<string, TreeNode>): TreeNode[] {
  return useMemo(() => {
    // Inline copy of treeAsArray logic to avoid a circular import
    // (ActivityTree.tsx imports types, this file imports ActivityTree).
    const byID = new Map<string, TreeNode & { children: TreeNode[] }>();
    for (const node of raw.values()) {
      byID.set(node.taskID, { ...node, children: [] });
    }
    const roots: TreeNode[] = [];
    for (const node of byID.values()) {
      if (!node.parentID || node.parentID === "root") {
        roots.push(node);
      } else {
        const parent = byID.get(node.parentID);
        if (parent) parent.children.push(node);
        else roots.push(node);
      }
    }
    roots.sort((a, b) => a.startedAt - b.startedAt);
    return roots;
  }, [raw]);
}
