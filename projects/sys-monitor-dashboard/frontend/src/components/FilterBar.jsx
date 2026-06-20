/**
 * FilterBar — reusable filter toolbar for the log dashboard.
 *
 * Renders two controls:
 *   1. A log-level dropdown  (All | INFO | WARN | ERROR | DEBUG)
 *   2. A time-range selector (Last hour | 24h | 7d | All)
 *
 * The component is fully controlled by the parent: it receives the current
 * `level` / `range` values and notifies the parent of changes via the
 * `onChange` callback, which receives a partial `{ level?, range? }` patch.
 *
 * Props:
 *   level    {string}   Current level filter. One of LEVEL_OPTIONS values.
 *   range    {string}   Current time range. One of RANGE_OPTIONS values.
 *   onChange {function} Called with `{ level?, range? }` whenever a control
 *                       changes. Parent merges the patch into its state.
 */

// ---- Option definitions (single source of truth) ----

export const LEVEL_OPTIONS = [
  { value: "ALL", label: "All levels" },
  { value: "INFO", label: "INFO" },
  { value: "WARN", label: "WARN" },
  { value: "ERROR", label: "ERROR" },
  { value: "DEBUG", label: "DEBUG" },
];

export const RANGE_OPTIONS = [
  { value: "1h", label: "Last hour" },
  { value: "24h", label: "24h" },
  { value: "7d", label: "7d" },
  { value: "all", label: "All" },
];

// ---- Small presentational helper ----

/**
 * Field — wraps a <label> + control with consistent minimalist styling.
 *
 * Props:
 *   label    {string}  Visible label text.
 *   htmlFor  {string}  id of the associated control.
 *   children {node}    The control element.
 */
function Field({ label, htmlFor, children }) {
  return (
    <div className="flex flex-col gap-1">
      <label
        htmlFor={htmlFor}
        className="text-xs font-semibold uppercase tracking-wider text-slate-500"
      >
        {label}
      </label>
      {children}
    </div>
  );
}

// ---- Main component ----

export default function FilterBar({ level = "ALL", range = "all", onChange }) {
  const handleChange = (patch) => {
    if (typeof onChange === "function") onChange(patch);
  };

  return (
    <div
      className="card flex flex-wrap items-end gap-6 px-5 py-4"
      role="group"
      aria-label="Log filters"
    >
      {/* Log level dropdown */}
      <Field label="Level" htmlFor="filter-level">
        <select
          id="filter-level"
          value={level}
          onChange={(e) => handleChange({ level: e.target.value })}
          className="rounded-md border border-slate-200 bg-white px-3 py-2 text-sm
                     font-medium text-slate-900 shadow-sm transition-colors
                     hover:border-slate-300 focus:border-slate-400"
        >
          {LEVEL_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </Field>

      {/* Time range selector */}
      <Field label="Time range" htmlFor="filter-range">
        <select
          id="filter-range"
          value={range}
          onChange={(e) => handleChange({ range: e.target.value })}
          className="rounded-md border border-slate-200 bg-white px-3 py-2 text-sm
                     font-medium text-slate-900 shadow-sm transition-colors
                     hover:border-slate-300 focus:border-slate-400"
        >
          {RANGE_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </Field>
    </div>
  );
}
