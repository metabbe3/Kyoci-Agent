/**
 * StatsBar — horizontal row of StatCard components showing total counts for
 * each log level (INFO, WARN, ERROR, DEBUG) fetched from the stats endpoint.
 *
 * Responsibilities:
 *   - Call `fetchStats()` on mount (and expose a manual refresh).
 *   - Normalize the backend payload into a stable per-level count map.
 *   - Render one StatCard per supported level with pastel accent colors.
 *
 * Expected `fetchStats()` payload shapes (any of these is supported):
 *   { counts: { INFO: n, WARN: n, ERROR: n, DEBUG: n } }
 *   { levels: { INFO: n, WARN: n, ERROR: n, DEBUG: n } }
 *   { INFO: n, WARN: n, ERROR: n, DEBUG: n }   // flat
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import StatCard from './StatCard.jsx';
import { fetchStats, ApiError } from '../api/client.js';

// ---------------------------------------------------------------------------
// Level configuration — single source of truth for order + accent colors.
// Pastel accents per the aesthetic constraint (high-contrast white bg).
// ---------------------------------------------------------------------------

const LEVELS = [
  { key: 'INFO', label: 'Info', accentColor: '#bfdbfe' }, // pastel blue
  { key: 'WARN', label: 'Warnings', accentColor: '#fde68a' }, // pastel yellow
  { key: 'ERROR', label: 'Errors', accentColor: '#fecaca' }, // pastel red
  { key: 'DEBUG', label: 'Debug', accentColor: '#e9d5ff' }, // pastel purple
];

// ---------------------------------------------------------------------------
// Payload normalization
// ---------------------------------------------------------------------------

/**
 * Coerce a stats payload of unknown shape into `{ [LEVEL]: count }`.
 *
 * Accepts `{ counts: {...} }`, `{ levels: {...} }`, or a flat level map.
 * Missing levels default to 0; non-numeric values are coerced to 0.
 *
 * @param {object|null|undefined} data
 * @returns {Record<string, number>}
 */
function normalizeCounts(data) {
  const source =
    (data && (data.counts || data.levels)) ||
    (data && typeof data === 'object' ? data : null) ||
    {};

  const counts = {};
  for (const { key } of LEVELS) {
    const raw = source[key];
    counts[key] = Number.isFinite(Number(raw)) ? Number(raw) : 0;
  }
  return counts;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * @param {object} props
 * @param {() => void} [props.onRefresh] - Optional callback after a successful refresh.
 * @param {string}   [props.className]   - Extra class for the row container.
 */
export default function StatsBar({ onRefresh, className = '' }) {
  const [counts, setCounts] = useState(() =>
    LEVELS.reduce((acc, { key }) => ({ ...acc, [key]: 0 }), {})
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await fetchStats();
      setCounts(normalizeCounts(data));
      if (typeof onRefresh === 'function') onRefresh();
    } catch (err) {
      const message =
        err instanceof ApiError
          ? err.isNetworkError
            ? 'Unable to reach the stats service.'
            : err.message
          : 'Failed to load stats.';
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [onRefresh]);

  // Fetch on mount.
  useEffect(() => {
    refresh();
  }, [refresh]);

  const total = useMemo(
    () => LEVELS.reduce((sum, { key }) => sum + (counts[key] || 0), 0),
    [counts]
  );

  return (
    <section
      className={`stats-bar ${className}`.trim()}
      aria-label="Log level statistics"
    >
      <div className="stats-bar__row" role="list">
        {LEVELS.map(({ key, label, accentColor }) => (
          <div role="listitem" key={key}>
            <StatCard
              title={label}
              count={counts[key] || 0}
              level={key}
              accentColor={accentColor}
            />
          </div>
        ))}
      </div>

      <div className="stats-bar__meta">
        <span className="stats-bar__total" aria-label="total log entries">
          {total.toLocaleString()} total entries
        </span>

        {loading ? (
          <span className="stats-bar__status stats-bar__status--loading">
            Updating…
          </span>
        ) : error ? (
          <span
            className="stats-bar__status stats-bar__status--error"
            role="alert"
          >
            {error}{' '}
            <button
              type="button"
              className="stats-bar__retry"
              onClick={refresh}
            >
              Retry
            </button>
          </span>
        ) : null}
      </div>
    </section>
  );
}
