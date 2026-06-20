import { exec } from 'child_process';
import { promisify } from 'util';
import os from 'os';

const execAsync = promisify(exec);

export interface ProcessInfo {
  pid: number;
  name: string;
  cpuPercent: number;
  memoryMB: number;
}

export type ProcessSortKey = 'cpu' | 'memory' | 'pid' | 'name';

export interface ScanOptions {
  sort?: ProcessSortKey;
  order?: 'asc' | 'desc';
  limit?: number;
}

/**
 * Parse a process line from `ps` output (Linux/Mac).
 * Expected columns: pid, %cpu, rss (kilobytes), comm/command.
 */
function parsePsLine(line: string): ProcessInfo | null {
  const trimmed = line.trim();
  if (!trimmed) return null;

  // Format: "pid  %cpu  rss  command..."
  const match = trimmed.match(/^(\d+)\s+([\d.]+)\s+(\d+)\s+(.+)$/);
  if (!match) return null;

  const pid = parseInt(match[1], 10);
  const cpuPercent = parseFloat(match[2]);
  const memoryMB = parseFloat(match[3]) / 1024; // KB -> MB
  const name = match[4].trim();

  if (Number.isNaN(pid) || Number.isNaN(cpuPercent) || Number.isNaN(memoryMB)) {
    return null;
  }

  return { pid, name, cpuPercent, memoryMB };
}

/**
 * Parse a line from Windows `wmic` / `tasklist` output.
 * tasklist /FO CSV /NH format: "Name,PID,MemUsage,..."
 */
function parseTasklistLine(line: string): ProcessInfo | null {
  const trimmed = line.trim();
  if (!trimmed) return null;

  // CSV: "chrome.exe","1234","45,678 K",...
  const parts = trimmed.match(/"([^"]*)"/g);
  if (!parts || parts.length < 3) return null;

  const name = parts[0].replace(/"/g, '');
  const pid = parseInt(parts[1].replace(/"/g, ''), 10);
  const memStr = parts[2].replace(/["\s,]/g, '').replace(/K$/i, '');
  const memoryMB = parseFloat(memStr) / 1024; // KB -> MB

  if (Number.isNaN(pid) || Number.isNaN(memoryMB)) return null;

  return { pid, name, cpuPercent: 0, memoryMB };
}

async function scanUnix(): Promise<ProcessInfo[]> {
  // pid, cpu%, rss (KB), command
  const { stdout } = await execAsync('ps -A -o pid,%cpu,rss,comm');
  return stdout
    .split('\n')
    .slice(1) // skip header
    .map(parsePsLine)
    .filter((p): p is ProcessInfo => p !== null);
}

async function scanWindows(): Promise<ProcessInfo[]> {
  const { stdout } = await execAsync('tasklist /FO CSV /NH');
  return stdout
    .split('\n')
    .map(parseTasklistLine)
    .filter((p): p is ProcessInfo => p !== null);
}

/**
 * Enumerate running processes with PID, name, CPU%, and memory (MB).
 * Uses `ps` on Linux/Mac and `tasklist` on Windows.
 */
export async function scanProcesses(): Promise<ProcessInfo[]> {
  try {
    return process.platform === 'win32' ? await scanWindows() : await scanUnix();
  } catch (err) {
    // Fallback: at minimum report the current process
    const mem = process.memoryUsage();
    return [
      {
        pid: process.pid,
        name: 'node',
        cpuPercent: 0,
        memoryMB: mem.rss / (1024 * 1024),
      },
    ];
  }
}

/**
 * Sort processes by the given key and order, optionally limiting the result count.
 */
export function sortProcesses(
  processes: ProcessInfo[],
  options: ScanOptions = {},
): ProcessInfo[] {
  const { sort = 'cpu', order = 'desc', limit } = options;

  const sorted = [...processes].sort((a, b) => {
    let cmp: number;
    switch (sort) {
      case 'memory':
        cmp = a.memoryMB - b.memoryMB;
        break;
      case 'pid':
        cmp = a.pid - b.pid;
        break;
      case 'name':
        cmp = a.name.localeCompare(b.name);
        break;
      case 'cpu':
      default:
        cmp = a.cpuPercent - b.cpuPercent;
        break;
    }
    return order === 'asc' ? cmp : -cmp;
  });

  return typeof limit === 'number' && limit > 0 ? sorted.slice(0, limit) : sorted;
}

/**
 * Convenience: scan and return a sorted (CPU desc by default) array of processes.
 */
export async function getProcesses(options: ScanOptions = {}): Promise<ProcessInfo[]> {
  const processes = await scanProcesses();
  return sortProcesses(processes, options);
}

// Re-export os for potential consumers needing host context
export { os };
