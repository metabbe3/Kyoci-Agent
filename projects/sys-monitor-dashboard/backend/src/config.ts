/**
 * Central configuration for the System Monitor backend.
 *
 * Values can be overridden via environment variables so the same image can run
 * in multiple environments without code changes.
 */

function envInt(name: string, fallback: number): number {
  const raw = process.env[name];
  if (raw === undefined || raw === null || raw === "") {
    return fallback;
  }
  const parsed = Number.parseInt(raw, 10);
  return Number.isNaN(parsed) ? fallback : parsed;
}

function envList(name: string, fallback: string[]): string[] {
  const raw = process.env[name];
  if (!raw) {
    return fallback;
  }
  return raw
    .split(",")
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
}

/** Percentage (0-100) above which an alert is raised. */
export interface AlertThresholds {
  /** CPU utilization percentage that triggers a warning. */
  cpu: number;
  /** Memory utilization percentage that triggers a warning. */
  memory: number;
  /** Disk utilization percentage that triggers a warning. */
  disk: number;
}

export interface AppConfig {
  /** Port the Express + WebSocket server listens on. */
  port: number;
  /** Interval (ms) between system metric samples / broadcasts. */
  pollIntervalMs: number;
  /** Allowed CORS origin patterns. */
  corsOrigins: string[];
  /** Thresholds (percentages) for raising alerts. */
  thresholds: AlertThresholds;
}

export const config: AppConfig = {
  port: envInt("PORT", 3001),
  pollIntervalMs: envInt("POLL_INTERVAL_MS", 2000),
  corsOrigins: envList("CORS_ORIGINS", [
    "http://localhost:5173",
    "http://localhost:3000",
  ]),
  thresholds: {
    cpu: 80,
    memory: 85,
    disk: 90,
  },
};

export default config;
