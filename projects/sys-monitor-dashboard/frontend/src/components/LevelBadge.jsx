/**
 * LevelBadge — small reusable pill that renders a log level with the
 * appropriate pastel background color.
 *
 * Props:
 *   - level: string  One of "ERROR", "WARN", "INFO", "DEBUG" (case-insensitive).
 *                   Unknown levels fall back to a neutral gray pill.
 *
 * The pastel tints are defined as utility classes in `src/index.css`
 * (`.badge-error`, `.badge-warn`, `.badge-info`, `.badge-debug`), keeping this
 * component presentational and free of inline style maps.
 */

const LEVEL_TO_CLASS = {
  ERROR: "badge-error",
  WARN: "badge-warn",
  WARNING: "badge-warn",
  INFO: "badge-info",
  DEBUG: "badge-debug",
};

const FALLBACK_CLASS = "badge-neutral";

/**
 * Resolve the CSS class for a given log level.
 *
 * @param {string} level - Raw log level string (any casing).
 * @returns {string} CSS class name from the badge-* family.
 */
function classForLevel(level) {
  if (typeof level !== "string") return FALLBACK_CLASS;
  const normalized = level.trim().toUpperCase();
  return LEVEL_TO_CLASS[normalized] ?? FALLBACK_CLASS;
}

/**
 * Normalize the display label so "warning" renders as "WARN", etc.
 *
 * @param {string} level - Raw log level string.
 * @returns {string} Uppercased canonical label.
 */
function labelForLevel(level) {
  if (typeof level !== "string" || level.trim() === "") return "UNKNOWN";
  const normalized = level.trim().toUpperCase();
  return normalized === "WARNING" ? "WARN" : normalized;
}

export default function LevelBadge({ level }) {
  const badgeClass = classForLevel(level);
  const label = labelForLevel(level);

  return (
    <span
      className={`level-badge ${badgeClass}`}
      role="status"
      aria-label={`Log level ${label}`}
    >
      {label}
    </span>
  );
}
