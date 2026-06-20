/**
 * MemoryBar — a stacked horizontal bar visualizing physical memory usage.
 *
 * Segments (left → right):
 *   1. Used      — memory actively in use by processes
 *   2. Buffer    — kernel buffers (derived: cached - cache, approximated)
 *   3. Cache     — cached / reclaimable memory
 *   4. Free      — unallocated memory
 *
 * The bar is driven by a `MemoryMetrics` snapshot. When the backend does not
 * split buffer from cache, the buffer segment collapses to zero width and only
 * used / cache / free are shown.
 */

import { useMemo } from 'react';
import type { MemoryMetrics } from '../types/metrics';

/** A single segment of the stacked bar. */
interface MemorySegment {
  /** Stable key used for React rendering and legend lookups. */
  key: 'used' | 'buffer' | 'cache' | 'free';
  /** Human-readable label shown in the legend. */
  label: string;
  /** Size of this segment in bytes. */
  bytes: number;
  /** Tailwind background class for the bar fill and legend swatch. */
  colorClass: string;
}

/** Human-friendly byte formatting (e.g. 8.2 GB, 512 MB). */
function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const exponent = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1,
  );
  const value = bytes / Math.pow(1024, exponent);
  const precision = exponent >= 2 ? 1 : 0;
  return `${value.toFixed(precision)} ${units[exponent]}`;
}

interface MemoryBarProps {
  /** Current memory snapshot. */
  memory: MemoryMetrics;
  /** Optional height override for the bar (px). Defaults to 28. */
  barHeightPx?: number;
}

/**
 * Decompose a MemoryMetrics snapshot into the four display segments.
 *
 * The backend reports `usedBytes`, `cachedBytes`, and `availableBytes`.
 * "Free" is the portion of available memory that is neither cached nor
 * already counted as used. "Buffer" is an optional split of the cache that
 * some platforms expose; when absent it is zero.
 */
function buildSegments(memory: MemoryMetrics): MemorySegment[] {
  const total = memory.totalBytes || 1; // guard against divide-by-zero
  const used = Math.max(0, memory.usedBytes);
  const cache = Math.max(0, memory.cachedBytes);

  // Available = free + reclaimable cache. Free is what remains after cache.
  const free = Math.max(0, memory.availableBytes - cache);

  // Buffer is not separately reported by the current backend; reserve the slot
  // so the legend stays stable if/when it is added.
  const buffer = 0;

  // Clamp so rounding never pushes the sum past total.
  const sum = used + buffer + cache + free;
  const scale = sum > total ? total / sum : 1;

  return [
    {
      key: 'used',
      label: 'Used',
      bytes: used * scale,
      colorClass: 'bg-rose-500',
    },
    {
      key: 'buffer',
      label: 'Buffer',
      bytes: buffer * scale,
      colorClass: 'bg-amber-400',
    },
    {
      key: 'cache',
      label: 'Cache',
      bytes: cache * scale,
      colorClass: 'bg-sky-400',
    },
    {
      key: 'free',
      label: 'Free',
      bytes: free * scale,
      colorClass: 'bg-emerald-500',
    },
  ];
}

export default function MemoryBar({
  memory,
  barHeightPx = 28,
}: MemoryBarProps): JSX.Element {
  const segments = useMemo(() => buildSegments(memory), [memory]);
  const total = memory.totalBytes || 0;

  const usedPercent = total > 0 ? (memory.usedBytes / total) * 100 : 0;

  return (
    <div className="w-full">
      {/* Header row: label + headline figures */}
      <div className="mb-2 flex items-baseline justify-between">
        <h3 className="text-sm font-semibold text-slate-200">Memory</h3>
        <span className="text-xs text-slate-400">
          {formatBytes(memory.usedBytes)} / {formatBytes(total)} ·{' '}
          <span
            className={
              usedPercent > 85
                ? 'font-semibold text-rose-400'
                : 'text-slate-300'
            }
          >
            {usedPercent.toFixed(1)}%
          </span>
        </span>
      </div>

      {/* Stacked horizontal bar */}
      <div
        className="flex w-full overflow-hidden rounded-md bg-slate-800 ring-1 ring-slate-700"
        style={{ height: `${barHeightPx}px` }}
        role="img"
        aria-label={`Memory usage: ${usedPercent.toFixed(1)} percent`}
      >
        {segments.map((segment) => {
          const widthPercent =
            total > 0 ? (segment.bytes / total) * 100 : 0;
          if (widthPercent <= 0) return null;
          return (
            <div
              key={segment.key}
              className={`${segment.colorClass} h-full transition-[width] duration-500 ease-out`}
              style={{ width: `${widthPercent}%` }}
              title={`${segment.label}: ${formatBytes(segment.bytes)}`}
            />
          );
        })}
      </div>

      {/* Legend */}
      <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-300">
        {segments.map((segment) => (
          <li key={segment.key} className="flex items-center gap-1.5">
            <span
              className={`inline-block h-2.5 w-2.5 rounded-sm ${segment.colorClass}`}
            />
            <span className="text-slate-400">{segment.label}</span>
            <span className="font-medium text-slate-200">
              {formatBytes(segment.bytes)}
            </span>
          </li>
        ))}
      </ul>

      {/* Swap indicator (if present) */}
      {memory.swapTotalBytes != null && memory.swapUsedBytes != null && (
        <div className="mt-2 text-xs text-slate-400">
          Swap:{' '}
          <span className="text-slate-200">
            {formatBytes(memory.swapUsedBytes)}
          </span>{' '}
          / {formatBytes(memory.swapTotalBytes)}
        </div>
      )}
    </div>
  );
}
