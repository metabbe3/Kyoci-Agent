/**
 * System information service.
 *
 * Gathers host-level metrics using the Node.js `os` module and platform
 * specific shell commands (via `child_process`) for disk usage.
 *
 * CPU utilization is derived by sampling `os.cpus()` over an interval and
 * comparing the deltas of idle/total ticks — the same technique `top` uses.
 */

import os, { CpuInfo } from "os";
import { exec } from "child_process";
import { promisify } from "util";

const execAsync = promisify(exec);

/* ------------------------------------------------------------------ *
 * Types
 * ------------------------------------------------------------------ */

export interface CpuCoreMetrics {
  /** 0-indexed core identifier. */
  index: number;
  /** Utilization percentage (0-100) for this core. */
  usage: number;
}

export interface CpuMetrics {
  /** Aggregate utilization across all cores (0-100). */
  usage: number;
  /** Per-core breakdown. */
  cores: CpuCoreMetrics[];
  /** Model name shared by all logical cores. */
  model: string;
  /** Number of logical cores. */
  count: number;
  /** 1 / 5 / 15 minute load averages. */
  loadAverage: [number, number, number];
}

export interface MemoryMetrics {
  totalBytes: number;
  freeBytes: number;
  usedBytes: number;
  /** Used as a percentage of total (0-100). */
  usage: number;
}

export interface DiskPartition {
  /** Mount point (Unix) or drive letter (Windows). */
  filesystem: string;
  /** Total capacity in bytes. */
  totalBytes: number;
  /** Used capacity in bytes. */
  usedBytes: number;
  /** Available capacity in bytes. */
  availableBytes: number;
  /** Used as a percentage of total (0-100). */
  usage: number;
}

export interface SystemInfo {
  hostname: string;
  /** Platform string, e.g. "linux", "darwin", "win32". */
  platform: string;
  uptimeSeconds: number;
  cpu: CpuMetrics;
  memory: MemoryMetrics;
  disks: DiskPartition[];
}

/* ------------------------------------------------------------------ *
 * CPU sampling
 * ------------------------------------------------------------------ */

interface CpuSample {
  cores: CpuInfo[];
  at: number;
}

let lastSample: CpuSample | null = null;

/**
 * Compute per-core and aggregate CPU usage by comparing two samples of
 * `os.cpus()`. The first call returns zeros because there is no baseline yet.
 */
function computeCpuUsage(prev: CpuSample, curr: CpuSample): CpuMetrics {
  const cores: CpuCoreMetrics[] = [];
  let totalIdleDelta = 0;
  let totalTickDelta = 0;

  for (let i = 0; i < curr.cores.length; i++) {
    const prevCore = prev.cores[i] ?? prev.cores[0];
    const currCore = curr.cores[i];

    const prevTotal =
      prevCore.times.user +
      prevCore.times.nice +
      prevCore.times.sys +
      prevCore.times.idle +
      prevCore.times.irq;
    const currTotal =
      currCore.times.user +
      currCore.times.nice +
      currCore.times.sys +
      currCore.times.idle +
      currCore.times.irq;

    const idleDelta = currCore.times.idle - prevCore.times.idle;
    const tickDelta = currTotal - prevTotal;
    const usage = tickDelta > 0 ? (1 - idleDelta / tickDelta) * 100 : 0;

    cores.push({ index: i, usage: clampPct(usage) });
    totalIdleDelta += idleDelta;
    totalTickDelta += tickDelta;
  }

  const aggregate =
    totalTickDelta > 0 ? (1 - totalIdleDelta / totalTickDelta) * 100 : 0;

  return {
    usage: clampPct(aggregate),
    cores,
    model: curr.cores[0]?.model ?? "unknown",
    count: curr.cores.length,
    loadAverage: loadAvgTuple(),
  };
}

function clampPct(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.max(0, Math.min(100, value));
}

function loadAvgTuple(): [number, number, number] {
  const [one, five, fifteen] = os.loadavg();
  return [one, five, fifteen];
}

/**
 * Capture a CPU sample and return the usage delta since the previous call.
 * Safe to call at any cadence; the first invocation seeds the baseline.
 */
function sampleCpu(): CpuMetrics {
  const sample: CpuSample = { cores: os.cpus(), at: Date.now() };
  const prev = lastSample;
  lastSample = sample;

  if (!prev || prev.cores.length !== sample.cores.length) {
    return {
      usage: 0,
      cores: sample.cores.map((_, i) => ({ index: i, usage: 0 })),
      model: sample.cores[0]?.model ?? "unknown",
      count: sample.cores.length,
      loadAverage: loadAvgTuple(),
    };
  }

  return computeCpuUsage(prev, sample);
}

/* ------------------------------------------------------------------ *
 * Memory
 * ------------------------------------------------------------------ */

function getMemory(): MemoryMetrics {
  const totalBytes = os.totalmem();
  const freeBytes = os.freemem();
  const usedBytes = totalBytes - freeBytes;
  const usage = totalBytes > 0 ? (usedBytes / totalBytes) * 100 : 0;
  return { totalBytes, freeBytes, usedBytes, usage: clampPct(usage) };
}

/* ------------------------------------------------------------------ *
 * Disk usage (platform specific)
 * ------------------------------------------------------------------ */

function parseUnixDf(stdout: string): DiskPartition[] {
  const lines = stdout.trim().split("\n");
  const partitions: DiskPartition[] = [];

  // Skip header line; columns: Filesystem Size Used Avail Use% MountedOn
  for (let i = 1; i < lines.length; i++) {
    const cols = lines[i].trim().split(/\s+/);
    if (cols.length < 6) continue;

    const filesystem = cols[0];
    const totalBytes = parseSize(cols[1]);
    const usedBytes = parseSize(cols[2]);
    const availableBytes = parseSize(cols[3]);
    if (totalBytes === 0) continue;

    const usage = (usedBytes / totalBytes) * 100;
    partitions.push({
      filesystem: cols.length > 6 ? cols.slice(5).join(" ") : filesystem,
      totalBytes,
      usedBytes,
      availableBytes,
      usage: clampPct(usage),
    });
  }

  return partitions;
}

/**
 * Convert a `df` size token (e.g. "64G", "512M", "1024") into bytes.
 * `df -h` uses K/M/G/T suffixes (base 1024).
 */
function parseSize(token: string): number {
  const match = /^([\d.]+)([KMGTPEZY]?)(i?)(B?)$/i.exec(token);
  if (!match) {
    const n = Number.parseFloat(token);
    return Number.isFinite(n) ? n : 0;
  }

  const value = Number.parseFloat(match[1]);
  const unit = match[2].toUpperCase();
  const base = match[3] === "i" || match[4] === "" ? 1024 : 1000;
  const exponent: Record<string, number> = {
    "": 0,
    K: 1,
    M: 2,
    G: 3,
    T: 4,
    P: 5,
    E: 6,
    Z: 7,
    Y: 8,
  };

  return value * Math.pow(base, exponent[unit] ?? 0);
}

function parseWindowsWmic(stdout: string): DiskPartition[] {
  const lines = stdout.trim().split(/\r?\n/).filter((l) => l.trim());
  const partitions: DiskPartition[] = [];

  for (const line of lines) {
    // wmic logicaldisk get returns space-separated columns
    const cols = line.trim().split(/\s+/);
    if (cols.length < 4) continue;

    const drive = cols[0];
    const freeBytes = Number(cols[1]);
    const totalBytes = Number(cols[2]);
    if (!Number.isFinite(totalBytes) || totalBytes === 0) continue;

    const usedBytes = totalBytes - (Number.isFinite(freeBytes) ? freeBytes : 0);
    const usage = (usedBytes / totalBytes) * 100;
    partitions.push({
      filesystem: drive,
      totalBytes,
      usedBytes,
      availableBytes: Number.isFinite(freeBytes) ? freeBytes : 0,
      usage: clampPct(usage),
    });
  }

  return partitions;
}

/**
 * Query disk usage. Uses `df` on Unix-like systems and `wmic` on Windows.
 * Returns an empty array if the command fails (e.g. restricted environment).
 */
async function getDisks(): Promise<DiskPartition[]> {
  try {
    if (process.platform === "win32") {
      const { stdout } = await execAsync(
        "wmic logicaldisk get Caption,FreeSpace,Size",
        { timeout: 5000 }
      );
      return parseWindowsWmic(stdout);
    }

    const { stdout } = await execAsync("df -k", { timeout: 5000 });
    return parseUnixDfKb(stdout);
  } catch {
    return [];
  }
}

/**
 * Parse `df -k` output (1K-blocks). Preferred over `df -h` because the
 * numeric values are unambiguous and trivially converted to bytes.
 */
function parseUnixDfKb(stdout: string): DiskPartition[] {
  const lines = stdout.trim().split("\n");
  const partitions: DiskPartition[] = [];

  for (let i = 1; i < lines.length; i++) {
    const cols = lines[i].trim().split(/\s+/);
    if (cols.length < 6) continue;

    const totalKb = Number(cols[1]);
    const usedKb = Number(cols[2]);
    const availKb = Number(cols[3]);
    if (!Number.isFinite(totalKb) || totalKb === 0) continue;

    partitions.push({
      filesystem: cols.slice(5).join(" ") || cols[0],
      totalBytes: totalKb * 1024,
      usedBytes: (Number.isFinite(usedKb) ? usedKb : 0) * 1024,
      availableBytes: (Number.isFinite(availKb) ? availKb : 0) * 1024,
      usage: clampPct((usedKb / totalKb) * 100),
    });
  }

  return partitions;
}

/* ------------------------------------------------------------------ *
 * Public API
 * ------------------------------------------------------------------ */

/**
 * Collect a full system snapshot. CPU usage reflects activity since the
 * previous call to this function (or `sampleCpu`), so callers should invoke
 * it on a regular interval for meaningful numbers.
 */
export async function getSystemInfo(): Promise<SystemInfo> {
  return {
    hostname: os.hostname(),
    platform: process.platform,
    uptimeSeconds: os.uptime(),
    cpu: sampleCpu(),
    memory: getMemory(),
    disks: await getDisks(),
  };
}

/**
 * Reset the CPU sampling baseline. Useful for tests or when the sampling
 * cadence changes dramatically.
 */
export function resetCpuBaseline(): void {
  lastSample = null;
}
