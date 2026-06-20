import * as os from 'os';
import * as fs from 'fs';
import * as path from 'path';

/**
 * Per-interface byte counters at a point in time.
 */
export interface InterfaceCounters {
  rxBytes: number;
  txBytes: number;
}

export type InterfaceCountersMap = Record<string, InterfaceCounters>;

/**
 * Throughput (bytes/sec) for a single interface, computed from the delta
 * between two consecutive counter samples divided by the elapsed seconds.
 */
export interface InterfaceThroughput {
  rxBytesPerSec: number;
  txBytesPerSec: number;
}

export type InterfaceThroughputMap = Record<string, InterfaceThroughput>;

/**
 * Reads cumulative rx/tx byte counters per network interface.
 *
 * On Linux it prefers /proc/net/dev, which exposes per-interface cumulative
 * byte counters directly. On other platforms (or if /proc/net/dev is
 * unavailable) it falls back to os.networkInterfaces(), which does not expose
 * byte counters — in that case the counters are reported as 0 and throughput
 * cannot be meaningfully computed (callers should treat 0 deltas as "unknown").
 */
export function readInterfaceCounters(): InterfaceCountersMap {
  if (process.platform === 'linux') {
    const parsed = readProcNetDev();
    if (parsed) {
      return parsed;
    }
  }
  return readFromOsInterfaces();
}

/**
 * Computes throughput (bytes/sec) for each interface present in BOTH samples.
 * Interfaces that appear in only one sample are skipped. A non-positive or
 * non-finite elapsedSeconds yields zero throughput.
 */
export function computeThroughput(
  previous: InterfaceCountersMap,
  current: InterfaceCountersMap,
  elapsedSeconds: number,
): InterfaceThroughputMap {
  const result: InterfaceThroughputMap = {};
  if (!elapsedSeconds || !isFinite(elapsedSeconds) || elapsedSeconds <= 0) {
    return result;
  }

  for (const [iface, cur] of Object.entries(current)) {
    const prev = previous[iface];
    if (!prev) {
      continue;
    }
    // Counters are cumulative; guard against counter resets (e.g. reboot/rebind)
    // which would produce a negative delta.
    const rxDelta = Math.max(0, cur.rxBytes - prev.rxBytes);
    const txDelta = Math.max(0, cur.txBytes - prev.txBytes);
    result[iface] = {
      rxBytesPerSec: rxDelta / elapsedSeconds,
      txBytesPerSec: txDelta / elapsedSeconds,
    };
  }
  return result;
}

/**
 * Parses /proc/net/dev into per-interface rx/tx byte counters.
 *
 * Format (Linux):
 *   Inter-|   Receive                                                |  Transmit
 *     face |bytes packets errs drop fifo frame compressed multicast|bytes packets ...
 *       lo: 1234    5   0   0    0     0          0         0  6789   10 ...
 *     eth0: ...
 *
 * Returns null if the file cannot be read or parsed.
 */
function readProcNetDev(): InterfaceCountersMap | null {
  const procPath = '/proc/net/dev';
  let content: string;
  try {
    content = fs.readFileSync(procPath, 'utf8');
  } catch {
    return null;
  }

  const lines = content.split('\n');
  // First two lines are headers.
  const dataLines = lines.slice(2).filter((l) => l.trim().length > 0);
  if (dataLines.length === 0) {
    return null;
  }

  const result: InterfaceCountersMap = {};
  for (const line of dataLines) {
    // "  eth0: 1234 5 ... 6789 10 ..."
    const colonIdx = line.indexOf(':');
    if (colonIdx === -1) {
      continue;
    }
    const iface = line.slice(0, colonIdx).trim();
    if (!iface) {
      continue;
    }
    const fields = line
      .slice(colonIdx + 1)
      .trim()
      .split(/\s+/)
      .map((f) => Number(f));
    // Receive bytes is field 0; Transmit bytes is field 8.
    const rxBytes = fields[0];
    const txBytes = fields[8];
    if (typeof rxBytes !== 'number' || typeof txBytes !== 'number') {
      continue;
    }
    result[iface] = { rxBytes, txBytes };
  }

  return Object.keys(result).length > 0 ? result : null;
}

/**
 * Fallback counter reader using os.networkInterfaces().
 *
 * Node's os.networkInterfaces() does not expose byte counters, so we return 0
 * for every interface. This keeps the interface list populated (useful for
 * display) while signaling that throughput is unavailable on this platform.
 */
function readFromOsInterfaces(): InterfaceCountersMap {
  const result: InterfaceCountersMap = {};
  const interfaces = os.networkInterfaces();
  for (const name of Object.keys(interfaces)) {
    const entries = interfaces[name];
    if (!entries) {
      continue;
    }
    // Only track interfaces that have at least one non-internal address.
    if (!entries.some((e) => !e.internal)) {
      continue;
    }
    result[name] = { rxBytes: 0, txBytes: 0 };
  }
  return result;
}

/**
 * Stateful monitor that tracks the previous counter sample and exposes the
 * latest throughput map. Call `tick()` once per poll interval.
 */
export class NetworkMonitor {
  private previous: InterfaceCountersMap | null = null;
  private previousTimestampMs: number | null = null;
  private latestThroughput: InterfaceThroughputMap = {};

  /** Returns the most recently computed throughput per interface. */
  getThroughput(): InterfaceThroughputMap {
    return this.latestThroughput;
  }

  /**
   * Samples the current counters and recomputes throughput against the
   * previous sample. Returns the freshly computed throughput map.
   */
  tick(): InterfaceThroughputMap {
    const now = Date.now();
    const current = readInterfaceCounters();

    if (this.previous && this.previousTimestampMs !== null) {
      const elapsedSeconds = (now - this.previousTimestampMs) / 1000;
      this.latestThroughput = computeThroughput(this.previous, current, elapsedSeconds);
    }

    this.previous = current;
    this.previousTimestampMs = now;
    return this.latestThroughput;
  }

  /** Resets stored state (e.g. after a long pause in sampling). */
  reset(): void {
    this.previous = null;
    this.previousTimestampMs = null;
    this.latestThroughput = {};
  }
}
