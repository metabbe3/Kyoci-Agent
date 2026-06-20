import { useMemo } from 'react';
import type { CpuMetrics } from '../types/metrics';

/**
 * Props for the CpuGauge component.
 */
export interface CpuGaugeProps {
  /** Aggregated CPU metrics. If null/undefined, the gauge renders a loading state. */
  cpu: CpuMetrics | null;
  /** Optional className for custom container styling. */
  className?: string;
}

// --- Geometry constants for the circular arc gauge ---
const SIZE = 180; // SVG viewBox size (px)
const STROKE_WIDTH = 14;
const RADIUS = (SIZE - STROKE_WIDTH) / 2;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;
// A 270° arc (3/4 circle) leaves a gap at the bottom for a cleaner look.
const ARC_FRACTION = 0.75;
const ARC_LENGTH = CIRCUMFERENCE * ARC_FRACTION;
const GAP_LENGTH = CIRCUMFERENCE - ARC_LENGTH;

/**
 * Returns a color along a green → yellow → red gradient based on a 0–100 value.
 */
function usageColor(value: number): string {
  const clamped = Math.max(0, Math.min(100, value));
  if (clamped < 50) {
    // green (#22c55e) → yellow (#eab308)
    const t = clamped / 50;
    return interpolateColor([34, 197, 94], [234, 179, 8], t);
  }
  // yellow (#eab308) → red (#ef4444)
  const t = (clamped - 50) / 50;
  return interpolateColor([234, 179, 8], [239, 68, 68], t);
}

function interpolateColor(a: number[], b: number[], t: number): string {
  const r = Math.round(a[0] + (b[0] - a[0]) * t);
  const g = Math.round(a[1] + (b[1] - a[1]) * t);
  const bl = Math.round(a[2] + (b[2] - a[2]) * t);
  return `rgb(${r}, ${g}, ${bl})`;
}

/**
 * Circular SVG arc gauge showing overall CPU utilization, with a per-core
 * breakdown rendered as horizontal bars beneath the gauge.
 */
export default function CpuGauge({ cpu, className }: CpuGaugeProps) {
  const usage = cpu?.usage ?? 0;
  const color = usageColor(usage);

  // The arc starts at 135° (bottom-left) and sweeps clockwise 270°.
  // We rotate the circle -90° so stroke-dashoffset maps to the visible arc.
  const dashOffset = useMemo(
    () => ARC_LENGTH - (ARC_LENGTH * usage) / 100,
    [usage],
  );

  return (
    <section
      className={`cpu-gauge ${className ?? ''}`.trim()}
      aria-label="CPU usage gauge"
    >
      <header className="cpu-gauge__header">
        <h3 className="cpu-gauge__title">CPU</h3>
        {cpu && (
          <span className="cpu-gauge__loadavg" title="1 / 5 / 15 min load average">
            load: {cpu.loadAvg1.toFixed(2)} / {cpu.loadAvg5.toFixed(2)} /{' '}
            {cpu.loadAvg15.toFixed(2)}
          </span>
        )}
      </header>

      <div className="cpu-gauge__dial">
        <svg
          width={SIZE}
          height={SIZE}
          viewBox={`0 0 ${SIZE} ${SIZE}`}
          role="img"
          aria-label={`CPU usage ${Math.round(usage)} percent`}
        >
          <g transform={`rotate(135 ${SIZE / 2} ${SIZE / 2})`}>
            {/* Track */}
            <circle
              cx={SIZE / 2}
              cy={SIZE / 2}
              r={RADIUS}
              fill="none"
              stroke="var(--gauge-track, rgba(255,255,255,0.08))"
              strokeWidth={STROKE_WIDTH}
              strokeLinecap="round"
              strokeDasharray={`${ARC_LENGTH} ${GAP_LENGTH}`}
            />
            {/* Progress */}
            <circle
              cx={SIZE / 2}
              cy={SIZE / 2}
              r={RADIUS}
              fill="none"
              stroke={color}
              strokeWidth={STROKE_WIDTH}
              strokeLinecap="round"
              strokeDasharray={`${ARC_LENGTH} ${GAP_LENGTH}`}
              strokeDashoffset={dashOffset}
              style={{ transition: 'stroke-dashoffset 0.6s ease, stroke 0.6s ease' }}
            />
          </g>
          {/* Center label */}
          <text
            x="50%"
            y="48%"
            textAnchor="middle"
            dominantBaseline="middle"
            className="cpu-gauge__value"
            fill={color}
          >
            {cpu ? Math.round(usage) : '--'}
          </text>
          <text
            x="50%"
            y="62%"
            textAnchor="middle"
            dominantBaseline="middle"
            className="cpu-gauge__unit"
            fill="currentColor"
          >
            % usage
          </text>
        </svg>
      </div>

      <div className="cpu-gauge__cores">
        <div className="cpu-gauge__cores-label">
          Per-core breakdown
          {cpu && (
            <span className="cpu-gauge__core-count">
              {cpu.coreCount} {cpu.coreCount === 1 ? 'core' : 'cores'}
            </span>
          )}
        </div>
        <ul className="cpu-gauge__core-list">
          {cpu?.cores?.length ? (
            cpu.cores.map((core) => (
              <li key={core.index} className="cpu-gauge__core-item">
                <span className="cpu-gauge__core-index" title={`Core ${core.index}`}>
                  {core.index}
                </span>
                <div
                  className="cpu-gauge__core-bar-track"
                  role="progressbar"
                  aria-valuenow={Math.round(core.usage)}
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-label={`Core ${core.index} usage`}
                >
                  <div
                    className="cpu-gauge__core-bar-fill"
                    style={{
                      width: `${Math.max(0, Math.min(100, core.usage))}%`,
                      backgroundColor: usageColor(core.usage),
                    }}
                  />
                </div>
                <span className="cpu-gauge__core-value">
                  {Math.round(core.usage)}%
                </span>
              </li>
            ))
          ) : (
            <li className="cpu-gauge__core-empty">No core data</li>
          )}
        </ul>
      </div>
    </section>
  );
}
