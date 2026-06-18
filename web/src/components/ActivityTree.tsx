import { useState } from "react";
import { motion, AnimatePresence } from "motion/react";
import type { TreeNode } from "../lib/types";

/**
 * ActivityTree — Claude-Code-style live activity log.
 *
 * Renders a list of TreeNode rows. Each row shows:
 *   ▸ Task name · N tool uses · M tokens · status pill
 *     ⎿  latest sub_activity detail
 *
 * On expand (▾), the full rolling sub-activity log shows under the row.
 * Compact mode (default in chat bubbles) shows only the latest sub-activity;
 * verbose mode (Live Activity panel) shows the full log by default.
 *
 * Visual conventions match the rest of the app: lime accents for running,
 * green check for done, coral for error. Status pills pulse while running.
 */
export function ActivityTree({
  nodes,
  compact = false,
  defaultExpanded = false,
  emptyHint,
}: {
  nodes: TreeNode[];
  /** Compact: only the latest sub-activity line per row. Default false. */
  compact?: boolean;
  /** All rows start expanded. Default false (collapsed in compact mode). */
  defaultExpanded?: boolean;
  /** What to render when there are no nodes. */
  emptyHint?: React.ReactNode;
}) {
  if (nodes.length === 0) {
    return emptyHint ? <>{emptyHint}</> : null;
  }
  return (
    <div className="flex flex-col gap-1.5 py-1">
      {nodes.map((node) => (
        <ActivityRow
          key={node.taskID}
          node={node}
          compact={compact}
          defaultExpanded={defaultExpanded}
          depth={0}
        />
      ))}
    </div>
  );
}

function ActivityRow({
  node,
  compact,
  defaultExpanded,
  depth,
}: {
  node: TreeNode;
  compact: boolean;
  defaultExpanded: boolean;
  depth: number;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded || !compact);
  const latest = node.subActivities.length > 0
    ? node.subActivities[node.subActivities.length - 1]
    : null;
  const elapsed = node.finishedAt
    ? `${((node.finishedAt - node.startedAt) / 1000).toFixed(1)}s`
    : `${((Date.now() - node.startedAt) / 1000).toFixed(1)}s`;

  const hasChildren = node.children.length > 0;
  const hasDetail = node.subActivities.length > 0;
  const expandable = hasDetail || hasChildren;

  return (
    <div style={{ marginLeft: depth * 18 }}>
      <div className="flex items-start gap-2 group">
        {/* Expand/collapse triangle (or dot when not expandable) */}
        <button
          type="button"
          onClick={() => expandable && setExpanded((e) => !e)}
          className="mt-0.5 w-4 flex-shrink-0 text-left opacity-60 hover:opacity-100"
          aria-label={expanded ? "Collapse" : "Expand"}
          disabled={!expandable}
        >
          {expandable ? (expanded ? "▾" : "▸") : "·"}
        </button>

        {/* Status pill */}
        <StatusPill status={node.status} />

        {/* Task name + metrics */}
        <div className="flex-1 min-w-0">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-sm">
            <span className="font-medium truncate" style={{ color: "var(--color-text-primary)" }}>
              {node.taskName}
            </span>
            <span className="opacity-50 text-xs">·</span>
            <span className="opacity-60 text-xs tabular-nums">
              {node.toolUses} tool{node.toolUses === 1 ? "" : "s"}
            </span>
            <span className="opacity-50 text-xs">·</span>
            <span className="opacity-60 text-xs tabular-nums">
              {formatTokens(node.tokensUsed)}
            </span>
            <span className="opacity-50 text-xs">·</span>
            <span className="opacity-50 text-xs tabular-nums">{elapsed}</span>
          </div>

          {/* Latest sub-activity (compact) OR full log (expanded) */}
          {!expanded && latest && (
            <div className="mt-0.5 pl-3 text-xs opacity-60 truncate" aria-label="Current activity">
              <span className="opacity-80 mr-1">⎿</span>
              {latest.detail || `${latest.tool_name} ${latest.tool_args || ""}`.trim()}
            </div>
          )}
        </div>
      </div>

      <AnimatePresence initial={false}>
        {expanded && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.18 }}
            className="overflow-hidden"
          >
            <ul className="mt-1 mb-1 pl-7 flex flex-col gap-0.5">
              {node.subActivities.map((evt, i) => (
                <li key={`${evt.timestamp}-${i}`} className="text-xs opacity-70 tabular-nums">
                  <span className="opacity-60 mr-1">⎿</span>
                  <span className="opacity-70 mr-1.5">
                    {new Date(evt.timestamp).toLocaleTimeString([], { hour12: false }).split(" ")[0]}
                  </span>
                  {evt.detail ||
                    `${evt.tool_name || ""} ${evt.tool_args || ""}`.trim() ||
                    "(no detail)"}
                </li>
              ))}
              {node.subActivities.length === 0 && (
                <li className="text-xs opacity-50 italic">(no tool calls yet)</li>
              )}
            </ul>
            {hasChildren && (
              <div className="mt-1">
                {node.children.map((child) => (
                  <ActivityRow
                    key={child.taskID}
                    node={child}
                    compact={compact}
                    defaultExpanded={defaultExpanded}
                    depth={depth + 1}
                  />
                ))}
              </div>
            )}
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

function StatusPill({ status }: { status: TreeNode["status"] }) {
  const color =
    status === "done"
      ? "var(--color-success, #10b981)"
      : status === "error"
      ? "var(--color-danger, #ef4444)"
      : "var(--color-lime, #84cc16)";
  const label = status === "done" ? "✓" : status === "error" ? "✗" : "●";
  return (
    <span
      className="mt-0.5 inline-flex h-3.5 w-3.5 flex-shrink-0 items-center justify-center rounded-full text-[9px]"
      style={{
        background: color,
        color: status === "running" ? "rgba(0,0,0,0.6)" : "white",
      }}
      aria-label={status}
    >
      {status === "running" ? (
        <motion.span
          animate={{ opacity: [1, 0.35, 1] }}
          transition={{ duration: 1.2, repeat: Infinity, ease: "easeInOut" }}
        >
          {label}
        </motion.span>
      ) : (
        label
      )}
    </span>
  );
}

function formatTokens(n: number): string {
  if (n <= 0) return "0 tokens";
  if (n < 1000) return `${n} tokens`;
  if (n < 100_000) return `${(n / 1000).toFixed(1)}k tokens`;
  return `${Math.round(n / 1000)}k tokens`;
}

/**
 * applyActivityEvent — pure reducer that folds an ActivityEvent into a tree
 * node map. Exported so useChatStream (chat-inline tree) and useActivityFeed
 * (Live Activity panel) share the same accumulation logic.
 */
export function applyActivityEvent(
  tree: Map<string, TreeNode>,
  evt: import("../lib/types").ActivityEvent
): Map<string, TreeNode> {
  const next = new Map(tree);
  const existing = next.get(evt.task_id);

  switch (evt.type) {
    case "task_start": {
      if (existing) {
        // Already exists (re-start?) — just update status.
        next.set(evt.task_id, { ...existing, status: "running", taskName: evt.task_name || existing.taskName });
      } else {
        next.set(evt.task_id, {
          taskID: evt.task_id,
          taskName: evt.task_name,
          parentID: evt.parent_id,
          role: evt.role,
          toolUses: 0,
          tokensUsed: 0,
          status: "running",
          startedAt: evt.timestamp,
          subActivities: [],
          children: [],
        });
      }
      break;
    }
    case "task_progress": {
      if (existing) {
        next.set(evt.task_id, {
          ...existing,
          toolUses: evt.tool_uses ?? existing.toolUses,
          tokensUsed: evt.tokens_used ?? existing.tokensUsed,
        });
      }
      break;
    }
    case "sub_activity": {
      if (existing) {
        const subs = [...existing.subActivities, evt];
        if (subs.length > 50) subs.shift();
        next.set(evt.task_id, { ...existing, subActivities: subs });
      }
      break;
    }
    case "task_complete": {
      if (existing) {
        next.set(evt.task_id, {
          ...existing,
          status: evt.status === "error" ? "error" : "done",
          toolUses: evt.tool_uses ?? existing.toolUses,
          tokensUsed: evt.tokens_used ?? existing.tokensUsed,
          finishedAt: evt.timestamp,
        });
      }
      break;
    }
    case "log": {
      if (existing) {
        const subs = [...existing.subActivities, evt];
        if (subs.length > 50) subs.shift();
        next.set(evt.task_id, { ...existing, subActivities: subs });
      }
      break;
    }
  }

  // Re-link children (delegation fan-out). Root nodes (no parentID) appear
  // at the top level; nodes with parentID get nested under their parent.
  return next;
}

/**
 * treeAsArray — flatten the task-id map into a top-level array of TreeNodes
 * with children nested. Root nodes (parentID empty or "root") are top-level;
 * everything else attaches to its parent's `children` array.
 */
export function treeAsArray(tree: Map<string, TreeNode>): TreeNode[] {
  const byID = new Map<string, TreeNode & { children: TreeNode[] }>();
  for (const node of tree.values()) {
    byID.set(node.taskID, { ...node, children: [] });
  }
  const roots: TreeNode[] = [];
  for (const node of byID.values()) {
    if (!node.parentID || node.parentID === "root") {
      roots.push(node);
    } else {
      const parent = byID.get(node.parentID);
      if (parent) parent.children.push(node);
      else roots.push(node); // orphan — promote to root
    }
  }
  // Sort by start time so newest is at the bottom (matches Claude Code).
  roots.sort((a, b) => a.startedAt - b.startedAt);
  return roots;
}
