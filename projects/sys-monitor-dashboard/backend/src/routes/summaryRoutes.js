/**
 * Express router exposing GET /api/summary.
 *
 * Purpose
 * -------
 * Produces a compact, privacy-safe "context window" of aggregated error
 * trends that an external/cloud summarizer can turn into an executive
 * post-mortem — WITHOUT ever exposing raw log lines, file paths, hostnames,
 * IPs, emails, or stack traces. Only counts and patterns leave the machine.
 *
 * Aggregations returned:
 *   - levelDistribution : total counts per log level (INFO/WARN/ERROR).
 *   - topErrorMessages  : most frequent normalized error message templates.
 *   - errorSpikeTimes   : hourly buckets ranked by error volume.
 *   - timeRange         : first/last log timestamps in the window.
 *
 * Privacy
 * -------
 * `maskMessage()` normalizes every message through a pipeline of redactors
 * (PII, secrets, paths, IPs, hex IDs, numbers) and then collapses runs of
 * redactions into a single `*` so structurally-identical messages hash to
 * the same template. The raw `message` / `raw_line` columns are never sent
 * over the API.
 *
 * All data stays strictly local; this endpoint only emits aggregates.
 */

import { Router } from 'express';
import getDb from '../db/connection.js';

const router = Router();

/* ------------------------------------------------------------------ *
 * Query tuning defaults
 * ------------------------------------------------------------------ */

/** How many top error templates to surface. */
const DEFAULT_TOP_N = 10;
/** How many spike buckets to surface. */
const DEFAULT_SPIKE_BUCKETS = 8;
/** Look-back window for the summary (SQLite `datetime` modifier). */
const DEFAULT_LOOKBACK_HOURS = 24;
/** Max length of a normalized message template before truncation. */
const MAX_TEMPLATE_LEN = 140;

/* ------------------------------------------------------------------ *
 * Privacy: message normalization / masking
 * ------------------------------------------------------------------ */

/**
 * Ordered list of [regexp, replacement] redactors.
 *
 * Order matters: more specific patterns run first so that, e.g., an email
 * is masked as a whole before the generic "word" or "number" pass would
 * shred it into fragments.
 *
 * Each pattern targets a class of sensitive or noisy token:
 *   - emails        : user@host
 *   - UUIDs         : 8-4-4-4-12 hex
 *   - IPv4 / IPv6   : numeric addresses
 *   - MAC addresses : aa:bb:cc:dd:ee:ff
 *   - hex hashes    : 32+ hex chars (sha1/md5/object refs)
 *   - long hex runs : 0x7ffe... / ffffff style tokens
 *   - secrets       : key=..., token=..., password=...
 *   - file paths    : /abs/path or C:\path
 *   - stack frames  : at foo (file.js:12:3)
 *   - numbers       : ids, ports, durations, memory sizes
 *
 * @type {Array<[RegExp, string]>}
 */
const REDACTORS = [
  [/\b[\w.+-]+@[\w-]+\.[\w.-]+\b/g, '<email>'],
  [/\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b/g, '<uuid>'],
  [/\b(?:\d{1,3}\.){3}\d{1,3}\b/g, '<ip>'],
  [/\b(?:[A-Fa-f0-9]{1,4}:){2,7}[A-Fa-f0-9]{1,4}\b/g, '<ip>'],
  [/\b(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}\b/g, '<mac>'],
  [/\b[0-9a-fA-F]{32,}\b/g, '<hash>'],
  [/\b0x[0-9a-fA-F]+\b/g, '<hex>'],
  [/\b(secret|password|passwd|token|api[_-]?key|access[_-]?key|auth)\s*[:=]\s*\S+/gi, '<secret>'],
  [/(?:\/[\w.-]+){2,}/g, '<path>'],
  [/\b[A-Za-z]:\\[^\s]+/g, '<path>'],
  [/\bat\s+[^\n]*\(\s*[\w./-]+:\d+:\d+\s*\)/g, ' <stack>'],
  [/\b\d+\b/g, '<n>'],
];

/**
 * Collapse adjacent redaction tokens into a single `*` and trim whitespace.
 *
 * After redaction, "user <email> failed <n> times from <ip>" becomes
 * "user * failed * times from *" — a stable template suitable for grouping.
 *
 * @param {string} text - Redacted text.
 * @returns {string} Collapsed template.
 */
function collapseRedactions(text) {
  return text
    .replace(/(?:<[^>]+>)+/g, '*')
    .replace(/\s*\*\s*/g, ' * ')
    .replace(/\*+/g, '*')
    .replace(/\s+/g, ' ')
    .trim();
}

/**
 * Normalize a raw log message into a privacy-safe, groupable template.
 *
 * Pipeline: lowercase → redact → collapse → truncate.
 * Returns an empty string for falsy / whitespace-only input.
 *
 * @param {string} raw - The extracted log message.
 * @returns {string} Masked template (e.g. "connection refused for *").
 */
function maskMessage(raw) {
  if (!raw || typeof raw !== 'string') return '';
  let out = raw.toLowerCase();
  for (const [re, repl] of REDACTORS) {
    out = out.replace(re, repl);
  }
  out = collapseRedactions(out);
  if (out.length > MAX_TEMPLATE_LEN) {
    out = `${out.slice(0, MAX_TEMPLATE_LEN - 1)}…`;
  }
  return out;
}

/* ------------------------------------------------------------------ *
 * Aggregation queries
 * ------------------------------------------------------------------ */

/**
 * Count logs grouped by level within the lookback window.
 *
 * @param {import('better-sqlite3').Database} db
 * @param {string} since - ISO timestamp lower bound.
 * @returns {Array<{level: string, count: number}>}
 */
function queryLevelDistribution(db, since) {
  const rows = db
    .prepare(
      `SELECT level, COUNT(*) AS count
       FROM logs
       WHERE timestamp >= ?
       GROUP BY level
       ORDER BY count DESC`,
    )
    .all(since);
  return rows.map((r) => ({ level: r.level, count: r.count }));
}

/**
 * Return the N most frequent masked error-message templates.
 *
 * We pull raw ERROR messages in-process and mask them in JS because SQLite
 * lacks the regex power to do the redaction pipeline. Counts are aggregated
 * post-mask so structurally identical errors collapse to one template.
 *
 * @param {import('better-sqlite3').Database} db
 * @param {string} since - ISO timestamp lower bound.
 * @param {number} topN - How many templates to return.
 * @returns {Array<{message: string, count: number}>}
 */
function queryTopErrorMessages(db, since, topN) {
  const rows = db
    .prepare(
      `SELECT message
       FROM logs
       WHERE level = 'ERROR' AND timestamp >= ?`,
    )
    .all(since);

  /** @type {Map<string, number>} */
  const counts = new Map();
  for (const { message } of rows) {
    const template = maskMessage(message);
    if (!template) continue;
    counts.set(template, (counts.get(template) ?? 0) + 1);
  }

  return [...counts.entries()]
    .map(([message, count]) => ({ message, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, topN);
}

/**
 * Rank hourly buckets by ERROR volume to surface spike times.
 *
 * Buckets are formatted as `YYYY-MM-DDTHH:00` (UTC) — no minutes/seconds,
 * which also thins out any timing-based fingerprinting.
 *
 * @param {import('better-sqlite3').Database} db
 * @param {string} since - ISO timestamp lower bound.
 * @param {number} limit - How many spike buckets to return.
 * @returns {Array<{hour: string, count: number}>}
 */
function queryErrorSpikes(db, since, limit) {
  const rows = db
    .prepare(
      `SELECT substr(timestamp, 1, 13) || ':00' AS hour,
              COUNT(*) AS count
       FROM logs
       WHERE level = 'ERROR' AND timestamp >= ?
       GROUP BY hour
       ORDER BY count DESC
       LIMIT ?`,
    )
    .all(since, limit);
  return rows.map((r) => ({ hour: r.hour, count: r.count }));
}

/**
 * First/last log timestamps in the window — gives the summarizer context
 * about how fresh/stale the data is.
 *
 * @param {import('better-sqlite3').Database} db
 * @param {string} since - ISO timestamp lower bound.
 * @returns {{first: string|null, last: string|null}}
 */
function queryTimeRange(db, since) {
  const row = db
    .prepare(
      `SELECT MIN(timestamp) AS first, MAX(timestamp) AS last
       FROM logs
       WHERE timestamp >= ?`,
    )
    .get(since);
  return { first: row?.first ?? null, last: row?.last ?? null };
}

/* ------------------------------------------------------------------ *
 * Route handler
 * ------------------------------------------------------------------ */

/**
 * Build the masked summary context window.
 *
 * @param {object} [opts]
 * @param {number} [opts.lookbackHours] - Look-back window in hours.
 * @param {number} [opts.topN]          - Max error templates to return.
 * @param {number} [opts.spikeBuckets]  - Max spike buckets to return.
 * @returns {object} Aggregated, masked summary payload.
 */
function buildSummary({
  lookbackHours = DEFAULT_LOOKBACK_HOURS,
  topN = DEFAULT_TOP_N,
  spikeBuckets = DEFAULT_SPIKE_BUCKETS,
} = {}) {
  const db = getDb();

  // Compute the `since` bound in SQLite so the DB engine does the math.
  const sinceRow = db
    .prepare(`SELECT strftime('%Y-%m-%dT%H:%M:%fZ', 'now', ?) AS since`)
    .get(`-${Math.max(0, lookbackHours)} hours`);
  const since = sinceRow?.since ?? '1970-01-01T00:00:00.000Z';

  const [levelDistribution, topErrorMessages, errorSpikeTimes, timeRange] = [
    queryLevelDistribution(db, since),
    queryTopErrorMessages(db, since, topN),
    queryErrorSpikes(db, since, spikeBuckets),
    queryTimeRange(db, since),
  ];

  const totalErrors = levelDistribution.find((l) => l.level === 'ERROR')?.count ?? 0;
  const totalWarns = levelDistribution.find((l) => l.level === 'WARN')?.count ?? 0;
  const totalInfo = levelDistribution.find((l) => l.level === 'INFO')?.count ?? 0;
  const totalLogs = totalErrors + totalWarns + totalInfo;

  return {
    generatedAt: new Date().toISOString(),
    windowHours: lookbackHours,
    timeRange,
    totals: {
      logs: totalLogs,
      errors: totalErrors,
      warnings: totalWarns,
      info: totalInfo,
    },
    levelDistribution,
    topErrorMessages,
    errorSpikeTimes,
    // A ready-to-paste prompt preamble for the cloud summarizer.
    // Contains ONLY counts + masked templates — no raw data.
    contextForAI: buildAIContext({
      windowHours: lookbackHours,
      timeRange,
      totals: { logs: totalLogs, errors: totalErrors, warnings: totalWarns },
      levelDistribution,
      topErrorMessages,
      errorSpikeTimes,
    }),
  };
}

/**
 * Render a compact, masked text blob suitable for an LLM post-mortem prompt.
 *
 * @param {object} ctx - Pre-aggregated, already-masked fields.
 * @returns {string} Plain-text context window.
 */
function buildAIContext({
  windowHours,
  timeRange,
  totals,
  levelDistribution,
  topErrorMessages,
  errorSpikeTimes,
}) {
  const lines = [];
  lines.push(`SYSTEM HEALTH SUMMARY (last ${windowHours}h)`);
  if (timeRange?.first && timeRange?.last) {
    lines.push(`window: ${timeRange.first} → ${timeRange.last}`);
  }
  lines.push(
    `totals: ${totals.logs} logs | ${totals.errors} errors | ${totals.warnings} warnings`,
  );
  lines.push(`levels: ${levelDistribution.map((l) => `${l.level}=${l.count}`).join(', ')}`);

  if (topErrorMessages.length) {
    lines.push('top error patterns:');
    for (const { message, count } of topErrorMessages) {
      lines.push(`  - (${count}x) ${message}`);
    }
  }
  if (errorSpikeTimes.length) {
    lines.push('error spikes (hour UTC):');
    for (const { hour, count } of errorSpikeTimes) {
      lines.push(`  - ${hour}: ${count} errors`);
    }
  }
  return lines.join('\n');
}

/**
 * Parse a non-negative integer query param with a fallback default.
 *
 * @param {string|undefined} raw - Raw query string value.
 * @param {number} fallback - Default when missing/invalid.
 * @returns {number}
 */
function parseIntParam(raw, fallback) {
  if (raw == null || raw === '') return fallback;
  const n = Number.parseInt(raw, 10);
  return Number.isFinite(n) && n >= 0 ? n : fallback;
}

/**
 * GET /api/summary
 *
 * Query params (all optional):
 *   - hours         : look-back window in hours   (default 24)
 *   - topN          : max error templates returned (default 10)
 *   - spikeBuckets  : max spike buckets returned   (default 8)
 *
 * @param {import('express').Request} req
 * @param {import('express').Response} res
 */
function getSummary(req, res) {
  try {
    const lookbackHours = Math.min(parseIntParam(req.query.hours, DEFAULT_LOOKBACK_HOURS), 720);
    const topN = Math.min(parseIntParam(req.query.topN, DEFAULT_TOP_N), 50);
    const spikeBuckets = Math.min(parseIntParam(req.query.spikeBuckets, DEFAULT_SPIKE_BUCKETS), 48);

    const summary = buildSummary({ lookbackHours, topN, spikeBuckets });
    res.json(summary);
  } catch (err) {
    // Never leak internal details — return a generic 500.
    console.error('[summaryRoutes] failed to build summary:', err);
    res.status(500).json({ error: 'failed_to_build_summary' });
  }
}

router.get('/api/summary', getSummary);

/* ------------------------------------------------------------------ *
 * Exports
 * ------------------------------------------------------------------ */

export default router;

// Exported for unit testing.
export {
  maskMessage,
  collapseRedactions,
  buildSummary,
  buildAIContext,
  REDACTORS,
  DEFAULT_LOOKBACK_HOURS,
  DEFAULT_TOP_N,
  DEFAULT_SPIKE_BUCKETS,
};
