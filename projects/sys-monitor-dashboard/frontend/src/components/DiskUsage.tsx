import { useMemo } from 'react';
import { HardDrive, AlertTriangle } from 'lucide-react';
import type { DiskMetrics } from '../types/metrics';

interface DiskUsageProps {
  /** Per-partition disk metrics to render. */
  disks: DiskMetrics[];
}

/** Threshold (percent) above which a partition is considered critical. */
const CRITICAL_THRESHOLD = 90;
/** Threshold (percent) above which a partition is considered in warning state. */
const WARNING_THRESHOLD = 75;

/**
 * Classify a usage percentage into a severity bucket for color coding.
 */
function severityFor(usagePercent: number): 'normal' | 'warning' | 'critical' {
  if (usagePercent >= CRITICAL_THRESHOLD) return 'critical';
  if (usagePercent >= WARNING_THRESHOLD) return 'warning';
  return 'normal';
}

const SEVERITY_COLORS: Record<'normal' | 'warning' | 'critical', string> = {
  normal: 'bg-emerald-500',
  warning: 'bg-amber-500',
  critical: 'bg-red-500',
};

const SEVERITY_TEXT: Record<'normal' | 'warning' | 'critical', string> = {
  normal: 'text-emerald-400',
  warning: 'text-amber-400',
  critical: 'text-red-400',
};

/**
 * Format a byte count into a human-readable string with binary units.
 */
function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.min(
    units.length - 1,
    Math.floor(Math.log(bytes) / Math.log(1024)),
  );
  const value = bytes / Math.pow(1024, i);
  const decimals = i === 0 ? 0 : value < 10 ? 1 : 0;
  return `${value.toFixed(decimals)} ${units[i]}`;
}

/**
 * Render a single partition row: mount label, usage bar, and stats.
 */
function PartitionRow({ disk }: { disk: DiskMetrics }) {
  const severity = severityFor(disk.usagePercent);
  const usedWidth = Math.min(100, Math.max(0, disk.usagePercent));

  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between text-sm">
        <div className="flex items-center gap-2 min-w-0">
          <HardDrive className="h-4 w-4 shrink-0 text-slate-400" />
          <span className="font-medium text-slate-200 truncate" title={disk.mount}>
            {disk.mount}
          </span>
          {disk.fsType && (
            <span className="text-xs text-slate-500 shrink-0">({disk.fsType})</span>
          )}
        </div>
        <span className={`font-semibold tabular-nums shrink-0 ${SEVERITY_TEXT[severity]}`}>
          {disk.usagePercent.toFixed(1)}%
        </span>
      </div>

      <div
        className="h-2.5 w-full overflow-hidden rounded-full bg-slate-700"
        role="progressbar"
        aria-valuenow={Number(disk.usagePercent.toFixed(1))}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={`Disk usage for ${disk.mount}`}
      >
        <div
          className={`h-full rounded-full transition-all duration-500 ease-out ${SEVERITY_COLORS[severity]}`}
          style={{ width: `${usedWidth}%` }}
        />
      </div>

      <div className="flex items-center justify-between text-xs text-slate-400 tabular-nums">
        <span>
          <span className="text-slate-300">{formatBytes(disk.usedBytes)}</span> used
        </span>
        <span>{formatBytes(disk.availableBytes)} free</span>
        <span className="text-slate-500">{formatBytes(disk.totalBytes)} total</span>
      </div>
    </div>
  );
}

/**
 * DiskUsage renders per-partition usage bars. Each partition shows its mount
 * point, a color-coded usage bar (green/amber/red based on thresholds), and
 * total, used, and available capacity with a percentage label.
 */
export default function DiskUsage({ disks }: DiskUsageProps) {
  const { totalBytes, usedBytes, availableBytes, overallPercent, criticalCount } =
    useMemo(() => {
      const total = disks.reduce((sum, d) => sum + d.totalBytes, 0);
      const used = disks.reduce((sum, d) => sum + d.usedBytes, 0);
      const available = disks.reduce((sum, d) => sum + d.availableBytes, 0);
      const percent = total > 0 ? (used / total) * 100 : 0;
      const critical = disks.filter((d) => d.usagePercent >= CRITICAL_THRESHOLD).length;
      return {
        totalBytes: total,
        usedBytes: used,
        availableBytes: available,
        overallPercent: percent,
        criticalCount: critical,
      };
    }, [disks]);

  if (disks.length === 0) {
    return (
      <div className="rounded-xl border border-slate-700 bg-slate-800/50 p-4">
        <div className="flex items-center gap-2 mb-3">
          <HardDrive className="h-5 w-5 text-slate-400" />
          <h3 className="text-sm font-semibold text-slate-200">Disk Usage</h3>
        </div>
        <p className="text-sm text-slate-500">No disk partitions available.</p>
      </div>
    );
  }

  return (
    <div className="rounded-xl border border-slate-700 bg-slate-800/50 p-4 space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <HardDrive className="h-5 w-5 text-slate-400" />
          <h3 className="text-sm font-semibold text-slate-200">Disk Usage</h3>
        </div>
        {criticalCount > 0 && (
          <div className="flex items-center gap-1.5 text-xs text-red-400">
            <AlertTriangle className="h-3.5 w-3.5" />
            <span>
              {criticalCount} {criticalCount === 1 ? 'partition' : 'partitions'} critical
            </span>
          </div>
        )}
      </div>

      {/* Aggregate summary */}
      <div className="flex items-center justify-between rounded-lg bg-slate-900/50 px-3 py-2 text-xs text-slate-400 tabular-nums">
        <span>
          Total:{' '}
          <span className="font-medium text-slate-300">{formatBytes(totalBytes)}</span>
        </span>
        <span>
          Used:{' '}
          <span className="font-medium text-slate-300">{formatBytes(usedBytes)}</span>
        </span>
        <span>
          Free:{' '}
          <span className="font-medium text-slate-300">
            {formatBytes(availableBytes)}
          </span>
        </span>
        <span className={`font-semibold ${SEVERITY_TEXT[severityFor(overallPercent)]}`}>
          {overallPercent.toFixed(1)}%
        </span>
      </div>

      {/* Per-partition bars */}
      <div className="space-y-3.5">
        {disks.map((disk) => (
          <PartitionRow key={disk.mount} disk={disk} />
        ))}
      </div>
    </div>
  );
}
