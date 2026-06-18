import { Component, type ErrorInfo, type ReactNode } from "react";

interface Props {
  children: ReactNode;
}
interface State {
  hasError: boolean;
  error?: Error;
}

/**
 * ErrorBoundary catches render-time errors anywhere in its subtree and shows a
 * recoverable fallback instead of a blank white screen. (React error boundaries
 * must be class components.) Wired around <App/> in main.tsx.
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Surface to the console for debugging (no external telemetry wired yet).
    console.error("[ErrorBoundary] render crash:", error, info.componentStack);
  }

  render() {
    if (this.state.hasError) {
      return (
        <div
          style={{
            padding: "2rem",
            fontFamily: "system-ui, sans-serif",
            color: "#f5f6f8",
            background: "#0d0f14",
            minHeight: "100vh",
            display: "flex",
            flexDirection: "column",
            alignItems: "flex-start",
            gap: "0.75rem",
          }}
        >
          <h2 style={{ margin: 0 }}>Something went wrong</h2>
          <p style={{ color: "#9aa0a8", margin: 0 }}>
            The UI hit an unexpected error. Reload the page to recover.
          </p>
          <pre
            style={{
              fontSize: "0.8rem",
              color: "#ff6b6b",
              whiteSpace: "pre-wrap",
              wordBreak: "break-word",
              margin: 0,
              maxWidth: "60ch",
            }}
          >
            {this.state.error?.message ?? "Unknown error"}
          </pre>
          <button
            onClick={() => window.location.reload()}
            style={{
              marginTop: "0.5rem",
              padding: "0.5rem 1rem",
              background: "#3b82f6",
              color: "white",
              border: "none",
              borderRadius: "6px",
              cursor: "pointer",
            }}
          >
            Reload
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
