/**
 * StatCard — reusable minimalist stat card.
 *
 * Displays a single metric (count) with a title, an optional log-level
 * label, and a colored accent dot that visually encodes severity.
 *
 * Props:
 *   title       {string}  Short label for the metric (e.g. "Errors").
 *   count       {number}  Numeric value to display prominently.
 *   level       {string=} Optional severity tag (INFO | WARN | ERROR).
 *   accentColor {string}  CSS color for the accent dot + left border.
 */
export default function StatCard({ title, count = 0, level, accentColor = "#9ca3af" }) {
  return (
    <div
      className="stat-card"
      style={{ borderLeftColor: accentColor }}
      role="group"
      aria-label={`${title} stat card`}
    >
      <div className="stat-card__header">
        <span
          className="stat-card__dot"
          style={{ backgroundColor: accentColor }}
          aria-hidden="true"
        />
        <span className="stat-card__title">{title}</span>
      </div>

      <div className="stat-card__count" aria-label={`${title} count`}>
        {count.toLocaleString()}
      </div>

      {level ? (
        <div className="stat-card__level" style={{ color: accentColor }}>
          {level}
        </div>
      ) : null}
    </div>
  );
}
