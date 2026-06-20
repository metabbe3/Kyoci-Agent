/**
 * ScanButton — reusable button that triggers a manual log-directory rescan.
 *
 * Calls `triggerScan()` from the API client and surfaces three visual states:
 *   • idle    — default, ready to scan.
 *   • loading — scan in progress (button disabled, spinner shown).
 *   • success — scan completed; briefly shows a checkmark + summary.
 *   • error   — scan failed; shows the error message with a retry affordance.
 *
 * The component is self-contained: it manages its own request lifecycle and
 * only needs an optional `onScanComplete` callback so the parent can refresh
 * its data (logs, stats, summary) after a successful scan.
 *
 * Props:
 *   onScanComplete {function=}  Called with the scan result object after a
 *                               successful scan. Parent typically refetches.
 *   onScanError    {function=}  Called with the ApiError if the scan fails.
 *   label          {string=}    Override the idle button label.
 *   className      {string=}    Extra classes appended to the button.
 *   disabled       {boolean=}   Hard-disable (e.g. when parent is busy).
 */

import { useCallback, useEffect, useRef, useState } from "react";
import { triggerScan, ApiError } from "../api/client.js";

// ---- Status constants (single source of truth) ----

const STATUS = Object.freeze({
  IDLE: "idle",
  LOADING: "loading",
  SUCCESS: "success",
  ERROR: "error",
});

/** How long (ms) the success state stays visible before reverting to idle. */
const SUCCESS_RESET_MS = 4000;

// ---- Small presentational helpers (kept local for cohesion) ----

/** Inline SVG spinner, sized via currentColor + width/height props. */
function Spinner({ className = "" }) {
  return (
    <svg
      className={`animate-spin ${className}`}
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
    >
      <circle
        className="opacity-25"
        cx="12"
        cy="12"
        r="10"
        stroke="currentColor"
        strokeWidth="4"
      />
      <path
        className="opacity-90"
        fill="currentColor"
        d="M4 12a8 8 0 0 1 8-8V0C5.4 0 0 5.4 0 12h4z"
      />
    </svg>
  );
}

/** Inline SVG checkmark for the success state. */
function CheckIcon({ className = "" }) {
  return (
    <svg
      className={className}
      viewBox="0 0 20 20"
      fill="currentColor"
      aria-hidden="true"
    >
      <path
        fillRule="evenodd"
        d="M16.7 5.3a1 1 0 0 1 0 1.4l-7.5 7.5a1 1 0 0 1-1.4 0L3.3 9.7a1 1 0 1 1 1.4-1.4l3.1 3.1 6.8-6.8a1 1 0 0 1 1.4 0z"
        clipRule="evenodd"
      />
    </svg>
  );
}

/** Inline SVG warning triangle for the error state. */
function WarnIcon({ className = "" }) {
  return (
    <svg
      className={className}
      viewBox="0 0 20 20"
      fill="currentColor"
      aria-hidden="true"
    >
      <path
        fillRule="evenodd"
        d="M8.5 2.5a1.5 1.5 0 0 1 2.6 0l6 11A1.5 1.5 0 0 1 15.7 16H3.3a1.5 1.5 0 0 1-1.3-2.2l6.5-11.3zM10 7a1 1 0 0 0-1 1v3a1 1 0 1 0 2 0V8a1 1 0 0 0-1-1zm0 7.2a1 1 0 1 0 0 2 1 1 0 0 0 0-2z"
        clipRule="evenodd"
      />
    </svg>
  );
}

// ---- Status → visual config mapping ----

/**
 * Centralized style/label config per status. Keeps the render body declarative
 * and makes it trivial to add a new status or tweak colors in one place.
 */
const STATUS_CONFIG = {
  [STATUS.IDLE]: {
    label: (label) => label,
    icon: null,
    buttonClass:
      "bg-slate-900 text-white hover:bg-slate-800 active:bg-slate-950",
    ariaLive: "polite",
  },
  [STATUS.LOADING]: {
    label: () => "Scanning…",
    icon: Spinner,
    iconClass: "h-4 w-4",
    buttonClass: "bg-slate-700 text-white cursor-wait",
    ariaLive: "polite",
  },
  [STATUS.SUCCESS]: {
    label: (result) => formatSuccessLabel(result),
    icon: CheckIcon,
    iconClass: "h-4 w-4",
    buttonClass: "bg-emerald-600 text-white hover:bg-emerald-700",
    ariaLive: "polite",
  },
  [STATUS.ERROR]: {
    label: () => "Retry scan",
    icon: WarnIcon,
    iconClass: "h-4 w-4",
    buttonClass:
      "bg-rose-600 text-white hover:bg-rose-700 active:bg-rose-800",
    ariaLive: "assertive",
  },
};

/**
 * Build a short human-readable label from the scan result payload.
 * Gracefully handles missing/unknown shapes.
 * @param {object|null} result
 * @returns {string}
 */
function formatSuccessLabel(result) {
  if (!result) return "Scan complete";
  const inserted =
    result.inserted ?? result.newLogs ?? result.count ?? result.scanned;
  if (typeof inserted === "number") {
    return `Scanned · ${inserted.toLocaleString()} new`;
  }
  return "Scan complete";
}

// ---- Main component ----

export default function ScanButton({
  onScanComplete,
  onScanError,
  label = "Rescan logs",
  className = "",
  disabled = false,
}) {
  const [status, setStatus] = useState(STATUS.IDLE);
  const [result, setResult] = useState(null);
  const [error, setError] = useState(null);

  // Ref so the timeout cleanup can survive re-renders without re-triggering.
  const resetTimerRef = useRef(null);

  /** Clear any pending success→idle reset timer. */
  const clearResetTimer = useCallback(() => {
    if (resetTimerRef.current !== null) {
      clearTimeout(resetTimerRef.current);
      resetTimerRef.current = null;
    }
  }, []);

  // Cleanup on unmount.
  useEffect(() => clearResetTimer, [clearResetTimer]);

  /** Execute the scan: sets loading, awaits triggerScan, transitions state. */
  const handleScan = useCallback(async () => {
    clearResetTimer();
    setStatus(STATUS.LOADING);
    setError(null);
    setResult(null);

    try {
      const scanResult = await triggerScan();
      setResult(scanResult);
      setStatus(STATUS.SUCCESS);
      if (typeof onScanComplete === "function") onScanComplete(scanResult);

      // Auto-revert to idle after a short delay so the button is reusable.
      resetTimerRef.current = setTimeout(() => {
        setStatus(STATUS.IDLE);
        setResult(null);
        resetTimerRef.current = null;
      }, SUCCESS_RESET_MS);
    } catch (err) {
      // Normalize non-ApiError throws so callers always get an ApiError.
      const normalized =
        err instanceof ApiError
          ? err
          : new ApiError(err?.message || "Scan failed unexpectedly", {
              endpoint: "/scan",
            });
      setError(normalized);
      setStatus(STATUS.ERROR);
      if (typeof onScanError === "function") onScanError(normalized);
    }
  }, [clearResetTimer, onScanComplete, onScanError]);

  const config = STATUS_CONFIG[status];
  const Icon = config.icon;
  const isBusy = status === STATUS.LOADING;
  const isDisabled = disabled || isBusy;

  // Compute the visible label from the active status config.
  const visibleLabel =
    status === STATUS.SUCCESS
      ? config.label(result)
      : status === STATUS.IDLE
        ? config.label(label)
        : config.label();

  return (
    <div className="scan-button inline-flex flex-col items-start gap-1">
      <button
        type="button"
        onClick={handleScan}
        disabled={isDisabled}
        aria-busy={isBusy}
        aria-live={config.ariaLive}
        className={`inline-flex items-center gap-2 rounded-md px-4 py-2 text-sm
                    font-semibold shadow-sm transition-colors duration-150
                    focus:outline-none focus-visible:ring-2
                    focus-visible:ring-offset-2 focus-visible:ring-slate-500
                    disabled:cursor-not-allowed disabled:opacity-60
                    ${config.buttonClass} ${className}`}
      >
        {Icon ? <Icon className={config.iconClass} /> : null}
        <span>{visibleLabel}</span>
      </button>

      {/* Inline error detail — only rendered in the error state. */}
      {status === STATUS.ERROR && error ? (
        <p
          role="alert"
          className="max-w-xs text-xs font-medium text-rose-600"
          title={error.endpoint ? `Endpoint: ${error.endpoint}` : undefined}
        >
          {error.isNetworkError
            ? "Could not reach the backend. Is it running?"
            : error.message}
        </p>
      ) : null}
    </div>
  );
}
