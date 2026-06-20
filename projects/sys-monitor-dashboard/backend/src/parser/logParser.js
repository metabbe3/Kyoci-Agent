/**
 * logParser.js — efficient regex-based parser for standard log line formats.
 *
 * Supported formats (auto-detected per line):
 *   1. ISO-8601 / syslog-ish:  "2024-05-01T12:34:56.789Z ERROR something broke"
 *   2. Bracketed timestamp:    "[2024-05-01 12:34:56] [ERROR] something broke"
 *   3. Common Log Format-ish:  "2024-05-01 12:34:56,789 ERROR something broke"
 *   4. Bare level prefix:      "ERROR something broke"  (timestamp unknown)
 *
 * Returns a structured object or null when the line is empty / unparseable.
 * All parsing is local; no data leaves the machine.
 */

'use strict';

/**
 * Canonical log levels we recognize. Stored upper-cased.
 * Anything outside this set is normalized to "UNKNOWN".
 */
export const KNOWN_LEVELS = new Set(['INFO', 'WARN', 'ERROR', 'DEBUG']);

/**
 * Normalize a raw level token (e.g. "warning", "err", "I") to the canonical form.
 * @param {string} raw
 * @returns {string}
 */
export function normalizeLevel(raw) {
  if (!raw) return 'UNKNOWN';
  const upper = raw.toUpperCase();

  // Short-form / common aliases.
  const aliases = {
    W: 'WARN',
    I: 'INFO',
    E: 'ERROR',
    D: 'DEBUG',
    ERR: 'ERROR',
    WARNING: 'WARN',
    FATAL: 'ERROR',
    CRITICAL: 'ERROR',
    CRIT: 'ERROR',
    TRACE: 'DEBUG',
    FINE: 'DEBUG',
  };
  if (aliases[upper]) return aliases[upper];
  return KNOWN_LEVELS.has(upper) ? upper : 'UNKNOWN';
}

/**
 * Parse a timestamp string into an ISO-8601 UTC string.
 * Returns null if the value cannot be parsed as a date.
 * @param {string} ts
 * @returns {string|null}
 */
function toISO(ts) {
  if (!ts) return null;
  // Replace space separator with T for ISO compatibility, keep fractional secs.
  const normalized = ts.replace(' ', 'T');
  const d = new Date(normalized);
  return Number.isNaN(d.getTime()) ? null : d.toISOString();
}

/**
 * Ordered list of compiled patterns. First match wins.
 * Using named groups keeps field extraction declarative.
 */
const PATTERNS = [
  // 1. ISO-8601 with optional zone: 2024-05-01T12:34:56.789Z  ERROR message
  {
    regex:
      /^(?<ts>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?)\s+(?<level>[A-Za-z]+)\s+(?<message>.*)$/,
  },
  // 2. Bracketed: [2024-05-01 12:34:56] [ERROR] message  (level optional brackets)
  {
    regex:
      /^\[(?<ts>\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?)\]\s*\[?(?<level>[A-Za-z]+)\]?\s+(?<message>.*)$/,
  },
  // 3. Space/comma separated: 2024-05-01 12:34:56,789 ERROR message
  {
    regex:
      /^(?<ts>\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?)\s+(?<level>[A-Za-z]+)\s+(?<message>.*)$/,
  },
  // 4. Bare level prefix (no timestamp): ERROR message
  {
    regex: /^(?<level>INFO|WARN(?:ING)?|ERROR|ERR|DEBUG|TRACE|FATAL|CRIT(?:ICAL)?)\s+(?<message>.*)$/i,
  },
];

/**
 * Parse a single log line into a structured object.
 *
 * @param {string} line — raw log line
 * @returns {{timestamp: string|null, level: string, message: string, raw: string}|null}
 *   - timestamp: ISO-8601 string, or null when not present/parseable
 *   - level:     canonical level (INFO|WARN|ERROR|DEBUG|UNKNOWN)
 *   - message:   trimmed message body
 *   - raw:       the original line (for traceability)
 *   Returns null for empty/whitespace-only lines.
 */
export function parseLine(line) {
  if (typeof line !== 'string') return null;
  const trimmed = line.trim();
  if (trimmed === '') return null;

  for (const { regex } of PATTERNS) {
    const m = regex.exec(trimmed);
    if (!m) continue;

    const groups = m.groups || {};
    const ts = groups.ts ? toISO(groups.ts) : null;
    const level = normalizeLevel(groups.level);
    const message = (groups.message || '').trim();

    return {
      timestamp: ts,
      level,
      message,
      raw: line,
    };
  }

  // Unrecognized format: keep the line but mark level UNKNOWN.
  return {
    timestamp: null,
    level: 'UNKNOWN',
    message: trimmed,
    raw: line,
  };
}

/**
 * Parse many lines at once, skipping nulls.
 * @param {string[]} lines
 * @returns {object[]}
 */
export function parseLines(lines) {
  if (!Array.isArray(lines)) return [];
  const out = [];
  for (const l of lines) {
    const parsed = parseLine(l);
    if (parsed) out.push(parsed);
  }
  return out;
}

export default { parseLine, parseLines, normalizeLevel, KNOWN_LEVELS };
