/**
 * Per-route error boundary for dashboard panels.
 *
 * The app-level {@link ErrorBoundary} (in `main.tsx`) catches anything that
 * escapes and shows a full-page fallback. This component wraps a single panel
 * so a render crash in (say) Providers doesn't blank the whole app — the user
 * keeps navigation and can recover by retrying or navigating away.
 *
 * Usage:
 *   <PanelBoundary name="Providers"><ProvidersPanel /></PanelBoundary>
 */

import { Component, type ErrorInfo, type ReactNode } from "react";
import { AlertTriangle, RefreshCw } from "lucide-react";
import { motion } from "motion/react";
import { springs } from "@/lib/motion";

interface Props {
  /** Human label shown in the fallback (e.g. "Providers"). */
  name: string;
  children: ReactNode;
}
interface State {
  hasError: boolean;
  error?: Error;
}

export class PanelBoundary extends Component<Props, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(`[PanelBoundary:${this.props.name}] render crash:`, error, info.componentStack);
  }

  private retry = () => {
    this.setState({ hasError: false, error: undefined });
  };

  render() {
    if (!this.state.hasError) return this.props.children;
    return (
      <motion.div
        initial={{ opacity: 0, y: 8 }}
        animate={{ opacity: 1, y: 0 }}
        transition={springs.gentle}
        className="glass-panel rounded-2xl p-8 mx-6 lg:mx-10 mt-10 max-w-2xl"
        role="alert"
      >
        <div className="flex items-start gap-4">
          <div
            className="h-10 w-10 grid place-items-center rounded-xl shrink-0"
            style={{
              background: "rgba(255, 107, 90, 0.12)",
              border: "1px solid rgba(255, 107, 90, 0.3)",
              color: "var(--color-coral)",
            }}
          >
            <AlertTriangle className="h-5 w-5" />
          </div>
          <div className="flex-1 min-w-0">
            <h3 className="text-base font-semibold text-[var(--color-ink)] mb-1">
              {this.props.name} failed to render
            </h3>
            <p className="text-sm text-[var(--color-ink-muted)] mb-3">
              An unexpected error occurred while drawing this panel. You can retry without leaving
              the page.
            </p>
            <pre className="text-xs font-mono text-[var(--color-coral)] bg-black/30 rounded-lg p-3 mb-4 whitespace-pre-wrap break-words">
              {this.state.error?.message ?? "Unknown error"}
            </pre>
            <button
              type="button"
              onClick={this.retry}
              data-cursor="hover"
              className="inline-flex items-center gap-1.5 rounded-lg border border-white/10 bg-white/5 px-3 py-1.5 text-xs font-medium text-[var(--color-ink)] hover:bg-white/10 transition-colors"
            >
              <RefreshCw className="h-3.5 w-3.5" />
              Retry
            </button>
          </div>
        </div>
      </motion.div>
    );
  }
}
