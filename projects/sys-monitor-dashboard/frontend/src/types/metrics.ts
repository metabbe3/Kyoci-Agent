// Core type definitions for the System Monitor Dashboard.
// These interfaces describe the shape of metrics data exchanged between the
// backend services/routes and the frontend hooks/components.

/** Severity level for an alert raised by the alert engine. */
export type AlertSeverity = 'info' | 'warning' | 'critical';

/** Per-core CPU information. */
export interface CpuCore {
  /** Zero-based core index. */
  index: number;
  /** Aggregate CPU utilization for this core, 0–100 (percent). */
  usage: number;
  /** Frequency in MHz, if available; otherwise null. */
  frequencyMHz: number | null;
}

/** Aggregated CPU metrics across all cores. */
export interface CpuMetrics {
  /** Average utilization across all cores, 0–100 (percent). */
  usage: number;
  /** Per-core breakdown. */
  cores: CpuCore[];
  /** Number of logical cores (os.cpus().length). */
  coreCount: number;
  /** 1-minute load average (platform-dependent). */
  loadAvg1: number;
  /** 5-minute load average. */
  loadAvg5: number;
  /** 15-minute load average. */
  loadAvg15: number;
}

/** Memory metrics in bytes. */
export interface MemoryMetrics {
  /** Total physical memory. */
  totalBytes: number;
  /** Memory currently in use. */
  usedBytes: number;
  /** Memory available for allocation (free + reclaimable cache). */
  availableBytes: number;
  /** Buffered / cached memory. */
  cachedBytes: number;
  /** Swap total; null if swap is disabled. */
  swapTotalBytes: number | null;
  /** Swap currently in use; null if swap is disabled. */
  swapUsedBytes: number | null;
  /** Convenience: used / total as a 0–100 percent. */
  usagePercent: number;
}

/** A single mounted filesystem / partition. */
export interface DiskMetrics {
  /** Mount point or device name (e.g. "/" or "C:"). */
  mount: string;
  /** Filesystem type (e.g. "ext4", "ntfs"); null if unknown. */
  fsType: string | null;
  /** Total capacity in bytes. */
  totalBytes: number;
  /** Used capacity in bytes. */
  usedBytes: number;
  /** Available capacity in bytes. */
  availableBytes: number;
  /** Used / total as a 0–100 percent. */
  usagePercent: number;
}

/** A single network interface and its throughput. */
export interface NetworkInterface {
  /** Interface name (e.g. "eth0", "Wi-Fi"). */
  name: string;
  /** MAC address if available; otherwise null. */
  mac: string | null;
  /** Whether the interface is currently up. */
  isUp: boolean;
  /** Total bytes received since boot. */
  rxBytes: number;
  /** Total bytes transmitted since boot. */
  txBytes: number;
  /** Receive throughput (bytes/sec) measured over the poll interval. */
  rxBytesPerSec: number;
  /** Transmit throughput (bytes/sec) measured over the poll interval. */
  txBytesPerSec: number;
}

/** Aggregated network metrics across all interfaces. */
export interface NetworkMetrics {
  interfaces: NetworkInterface[];
  /** Aggregate receive throughput across all interfaces (bytes/sec). */
  totalRxBytesPerSec: number;
  /** Aggregate transmit throughput across all interfaces (bytes/sec). */
  totalTxBytesPerSec: number;
}

/** A single process row in the process table. */
export interface ProcessInfo {
  /** Operating-system process ID. */
  pid: number;
  /** Process / command name. */
  name: string;
  /** CPU utilization, 0–100 (percent). */
  cpuPercent: number;
  /** Resident memory in megabytes (MB). */
  memoryMB: number;
  /** Owning user, if available; otherwise null. */
  user: string | null;
}

/**
 * A complete point-in-time snapshot of the system, broadcast over the
 * WebSocket and returned by GET /api/metrics.
 */
export interface SystemSnapshot {
  /** ISO-8601 timestamp the snapshot was collected. */
  timestamp: string;
  /** Machine hostname. */
  hostname: string;
  /** Platform string (e.g. "linux", "darwin", "win32"). */
  platform: string;
  /** System uptime in seconds. */
  uptimeSeconds: number;
  cpu: CpuMetrics;
  memory: MemoryMetrics;
  disks: DiskMetrics[];
  network: NetworkMetrics;
}

/** An alert raised by the alert engine when a threshold is exceeded. */
export interface Alert {
  /** Stable identifier for deduplication / dismissal. */
  id: string;
  /** Severity of the alert. */
  severity: AlertSeverity;
  /** Human-readable summary (e.g. "High CPU usage"). */
  title: string;
  /** Detailed description of the condition. */
  message: string;
  /** The metric category this alert pertains to. */
  category: 'cpu' | 'memory' | 'disk' | 'network' | 'system';
  /** Current measured value that triggered the alert. */
  currentValue: number;
  /** Threshold value that was exceeded. */
  threshold: number;
  /** ISO-8601 timestamp the alert was raised. */
  timestamp: string;
  /** Whether the condition is still active. */
  active: boolean;
}
