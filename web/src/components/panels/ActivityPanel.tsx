import { motion } from "motion/react";
import { Activity, Zap, CircleSlash } from "lucide-react";
import { TopBar } from "@/components/layout/TopBar";
import { ActivityTree } from "@/components/ActivityTree";
import { useActivityFeed } from "@/hooks/useActivityFeed";
import { springs, staggerContainer, staggerItem } from "@/lib/motion";

/**
 * ActivityPanel — Live Activity panel at /activity.
 *
 * Aggregates every running agent across the app into one Claude-Code-style
 * tree. Subscribes to /api/dashboard/activity SSE via the broker; events flow
 * in real-time from the orchestrator's workers + delegations + explore
 * sub-agents.
 *
 * Empty state ("No active agents") shows when nothing is running. Rows
 * auto-prune 60s after completion to keep the panel scannable.
 */
export function ActivityPanel() {
  const { tree, connected, subscriberCount } = useActivityFeed();

  const running = tree.filter((n) => n.status === "running").length;
  const done = tree.filter((n) => n.status === "done").length;
  const errored = tree.filter((n) => n.status === "error").length;

  return (
    <>
      <TopBar
        eyebrow="Live"
        title="Activity"
        subtitle={
          connected
            ? `${running} running · ${done} done${errored ? ` · ${errored} errored` : ""} · ${subscriberCount} subscriber${subscriberCount === 1 ? "" : "s"}`
            : "Reconnecting…"
        }
      />

      <motion.div
        variants={staggerContainer()}
        initial="initial"
        animate="animate"
        className="px-6 py-6 max-w-5xl"
      >
        {tree.length === 0 ? (
          <motion.div variants={staggerItem} className="flex flex-col items-center justify-center py-24 opacity-60">
            <CircleSlash className="h-8 w-8 mb-3 opacity-40" />
            <p className="text-sm">No active agents. Start a task in Chat.</p>
          </motion.div>
        ) : (
          <motion.div
            variants={staggerItem}
            className="rounded-2xl border border-white/10 bg-white/[0.02] p-4"
            transition={springs.gentle}
          >
            <div className="flex items-center gap-2 mb-3 text-xs uppercase tracking-wider opacity-60">
              <Zap className="h-3 w-3" />
              <span>{running > 0 ? `Running ${running} task${running === 1 ? "" : "s"}…` : "Recent activity"}</span>
            </div>
            <ActivityTree nodes={tree} compact={false} defaultExpanded />
          </motion.div>
        )}

        {!connected && (
          <motion.div
            variants={staggerItem}
            className="mt-4 rounded-xl border border-[var(--color-coral)]/30 bg-[var(--color-coral)]/5 px-4 py-2 text-xs text-[var(--color-coral)]"
          >
            Lost connection to /api/dashboard/activity. Reconnecting with backoff…
          </motion.div>
        )}
      </motion.div>
    </>
  );
}
