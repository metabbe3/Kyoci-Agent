/**
 * HealthSummary — executive post-mortem panel.
 *
 * Fetches the AI-generated system health summary from `/api/summary` and
 * renders it in a clean, scannable card layout:
 *
 *   ┌──────────────────────────────────────────────┐
 *   │  Health status badge (HEALTHY / DEGRADED / …)│
 *   │  Generated-at timestamp + context window     │
 *   ├──────────────────────────────────────────────┤
 *   │  Narrative summary paragraph                 │
 *   ├──────────────────────────────────────────────┤
 *   │  Top error patterns (ranked list)            │
 *   ├──────────────────────────────────────────────┤
 *   │  Recommendations (actionable checklist)      │
 *   └──────────────────────────────────────────────┘
 *
 * The component is resilient to partial payloads: any missing section is
 * simply omitted rather than rendering empty placeholders.
 *
 * Expected `fetchSummary()` payload (all fields optional):
 *   {
 *     status:        "HEALTHY" | "DEGRADED" | "CRITICAL" | string,
 *     summary:       string,                    // narrative paragraph
 *     topErrors:     [{ pattern, count, level }],
 *     recommendations: [string],
 *     generatedAt:   ISO-8601 string,
 *     windowHours:   number
 *   }
 */

import { useCallback, useEffect, useState } from 'react';
import { fetchSummary, ApiError } from '../api/client.js';

// ---------------------------------------------------------------------------
// Status configuration — single source of truth for badge styling.
// ---------------------------------------------------------------------------

/**
 * Maps a health-status keyword to its display metadata.
 * Unknown statuses fall back to a neutral appearance.
 */
const STATUS_CONFIG = {
  HEALTHY: { label: 'Healthy', className: 'health-badge--healthy', dot: '#86efac' },
  DEGRADED: { label: 'Degraded', className: 'health-badge--degraded', dot: '#fde68a' },
  CRITICAL: { label: 'Critical', className: 'health-badge--critical', dot: '#fecaca' },
  UNKNOWN: { label: 'Unknown', className: 'health-badge--unknown', dot: '#e5e7eb' },
};

/**
 * Resolve status metadata from a raw status string (any casing).
 * @param {string|undefined} status
 * @returns {{label: string, className: string, dot: string}}
 */
function resolveStatus(status) {
  if (typeof status !== 'string' || status.trim() === '') {
    return STATUS_CONFIG.UNKNOWN;
  }
  const normalized = status.trim().toUpperCase();
  return STATUS_CONFIG[normalized] ?? {
    label: normalized,
    className: STATUS_CONFIG.UNKNOWN.className,
    dot: STATUS_CONFIG.UNKNOWN.dot,
  };
}

// ---------------------------------------------------------------------------
// Payload normalization helpers
// ---------------------------------------------------------------------------

/**
 * Coerce the `topErrors` field into a stable array of
 * `{ pattern, count, level }` objects, sorted by count descending.
 *
 * Accepts arrays of strings, arrays of objects, or a `{ pattern: count }` map.
 *
 * @param {any} raw
 * @returns {Array<{pattern: string, count: number, level: string}>}
 */
function normalizeTopErrors(raw) {
  if (!raw) return [];

  // Object map: { "NullPointer at X": 42, ... }
  if (!Array.isArray(raw) && typeof raw === 'object') {
    return Object.entries(raw)
      .map(([pattern, count]) => ({
        pattern,
        count: Number(count) || 0,
        level: 'ERROR',
      }))
      .sort((a, b) => b.count - a.count);
  }

  if (!Array.isArray(raw)) return [];

  return raw
    .map((entry) => {
      if (typeof entry === 'string') {
        return { pattern: entry, count: 0, level: 'ERROR' };
      }
      if (entry && typeof entry === 'object') {
        return {
          pattern: String(entry.pattern ?? entry.message ?? entry.error ?? 'Unknown'),
          count: Number(entry.count ?? entry.occurrences ?? 0) || 0,
          level: String(entry.level ?? 'ERROR'),
        };
      }
      return null;
    })
    .filter(Boolean)
    .sort((a, b) => b.count - a.count);
}

/**
 * Coerce the `recommendations` field into a clean string array.
 *
 * @param {any} raw
 * @returns {string[]}
 */
function normalizeRecommendations(raw) {
  if (!raw) return [];
  if (Array.isArray(raw)) {
    return raw.map((r) => (typeof r === 'string' ? r : String(r?.text ?? r ?? ''))).filter(Boolean);
  }
  if (typeof raw === 'string') {
    return raw
      .split(/\n+/)
      .map((line) => line.replace(/^[-•*\d.)\s]+/, '').trim())
      .filter(Boolean);
  }
  return [];
}

/**
 * Format an ISO timestamp into a human-friendly local time string.
 * Returns the raw value if parsing fails.
 *
 * @param {string|undefined} iso
 * @returns {string}
 */
function formatTimestamp(iso) {
  if (!iso) return '';
  try {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return String(iso);
    return d.toLocaleString(undefined, {
      dateStyle: 'medium',
      timeStyle: 'short',
    });
  } catch {
    return String(iso);
  }
}

// ---------------------------------------------------------------------------
// Presentational sub-components
// ---------------------------------------------------------------------------

/**
 * HealthBadge — pill showing the overall system health status with a
 * colored dot and label.
 */
function HealthBadge({ status }) {
  const { label, className, dot } = resolveStatus(status);
  return (
    <span className={`health-badge ${className}`} role="status" aria-label={`System health: ${label}`}>
      <span className="health-badge__dot" style={{ backgroundColor: dot }} aria-hidden="true" />
      {label}
    </span>
  );
}

/**
 * ErrorPatternItem — a single ranked error-pattern row.
 */
function ErrorPatternItem({ pattern, count, level, rank }) {
  return (
    <li className="health-summary__error-item">
      <span className="health-summary__rank" aria-hidden="true">
        {rank}
      </span>
      <span className="health-summary__error-pattern" title={pattern}>
        {pattern}
      </span>
      {count > 0 && (
        <span className="health-summary__error-count" aria-label={`${count} occurrences`}>
          {count.toLocaleString()}
        </span>
      )}
    </li>
  );
}

/**
 * RecommendationItem — a single actionable recommendation row with a
 * checkmark glyph.
 */
function RecommendationItem({ children }) {
  return (
    <li className="health-summary__rec-item">
      <span className="health-summary__rec-check" aria-hidden="true">
        ✓
      </span>
      <span className="health-summary__rec-text">{children}</span>
    </li>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

/**
 * @param {object} props
 * @param {() => void} [props.onRefresh] - Optional callback after a successful refresh.
 * @param {string}    [props.className]  - Extra class for the panel container.
 */
export default function HealthSummary({ onRefresh, className = '' }) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const payload = await fetchSummary();
      setData(payload);
      if (typeof onRefresh === 'function') onRefresh();
    } catch (err) {
      const message =
        err instanceof ApiError
          ? err.isNetworkError
            ? 'Unable to reach the summary service.'
            : err.message
          : 'Failed to load health summary.';
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [onRefresh]);

  // Fetch on mount.
  useEffect(() => {
    refresh();
  }, [refresh]);

  // ---- Derived values (normalized once per render) ----
  const status = data?.status;
  const summaryText = data?.summary ? String(data.summary) : '';
  const topErrors = normalizeTopErrors(data?.topErrors ?? data?.errorPatterns ?? data?.patterns);
  const recommendations = normalizeRecommendations(
    data?.recommendations ?? data?.actions ?? data?.suggestions
  );
  const generatedAt = formatTimestamp(data?.generatedAt ?? data?.generated_at);
  const windowHours = Number(data?.windowHours ?? data?.window_hours) || null;

  // ---- Loading state ----
  if (loading && !data) {
    return (
      <section
        className={`health-summary health-summary--loading ${className}`.trim()}
        aria-label="System health summary"
        aria-busy="true"
      >
        <div className="health-summary__skeleton" />
      </section>
    );
  }

  // ---- Error state ----
  if (error && !data) {
    return (
      <section
        className={`health-summary health-summary--error ${className}`.trim()}
        aria-label="System health summary"
      >
        <div className="health-summary__error-state" role="alert">
          <p className="health-summary__error-text">{error}</p>
          <button type="button" className="health-summary__retry" onClick={refresh}>
            Retry
          </button>
        </div>
      </section>
    );
  }

  // ---- Empty state (no data at all) ----
  if (!data) {
    return (
      <section
        className={`health-summary health-summary--empty ${className}`.trim()}
        aria-label="System health summary"
      >
        <p className="health-summary__empty-text">No health summary available yet.</p>
      </section>
    );
  }

  // ---- Success render ----
  return (
    <section
      className={`health-summary ${className}`.trim()}
      aria-label="System health summary"
    >
      {/* Header: status badge + meta */}
      <header className="health-summary__header">
        <div className="health-summary__heading">
          <h2 className="health-summary__title">System Health</h2>
          <HealthBadge status={status} />
        </div>

        <div className="health-summary__meta">
          {generatedAt && (
            <span className="health-summary__meta-item" title="Summary generated at">
              Updated {generatedAt}
            </span>
          )}
          {windowHours && (
            <span className="health-summary__meta-item" title="Analysis window">
              Last {windowHours}h
            </span>
          )}
          <button
            type="button"
            className="health-summary__refresh"
            onClick={refresh}
            disabled={loading}
            aria-label="Refresh health summary"
          >
            {loading ? 'Refreshing…' : 'Refresh'}
          </button>
        </div>
      </header>

      {/* Narrative summary */}
      {summaryText && (
        <div className="health-summary__section">
          <p className="health-summary__narrative">{summaryText}</p>
        </div>
      )}

      {/* Top error patterns */}
      {topErrors.length > 0 && (
        <div className="health-summary__section">
          <h3 className="health-summary__section-title">Top Error Patterns</h3>
          <ol className="health-summary__error-list">
            {topErrors.map((err, idx) => (
              <ErrorPatternItem
                key={`${err.pattern}-${idx}`}
                pattern={err.pattern}
                count={err.count}
                level={err.level}
                rank={idx + 1}
              />
            ))}
          </ol>
        </div>
      )}

      {/* Recommendations */}
      {recommendations.length > 0 && (
        <div className="health-summary__section">
          <h3 className="health-summary__section-title">Recommendations</h3>
          <ul className="health-summary__rec-list">
            {recommendations.map((rec, idx) => (
              <RecommendationItem key={`rec-${idx}`}>{rec}</RecommendationItem>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
