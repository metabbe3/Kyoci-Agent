/**
 * Log directory configuration.
 *
 * Defines the list of directories the backend will scan for system/application
 * logs. The defaults can be overridden via the `LOG_DIRS` environment variable,
 * which accepts a colon-separated list of paths (POSIX-style, mirroring `$PATH`).
 *
 * Keeping this in its own module means every scanner/parser/importer reads from
 * a single source of truth, so adding a new watch directory never requires
 * touching business logic.
 */

'use strict';

const path = require('path');

/**
 * Built-in default directories to scan when LOG_DIRS is not set.
 *
 * - `/var/log` is the conventional system log location on most Unix-like OSes.
 * - `./sample-logs` is a project-local directory used for development and tests
 *   so the backend works out-of-the-box without requiring root privileges.
 */
const DEFAULT_LOG_DIRS = ['/var/log', './sample-logs'];

/**
 * Parse a colon-separated list of directories from an environment variable.
 *
 * Empty segments are filtered out so values like `"/var/log::/tmp"` collapse to
 * `["/var/log", "/tmp"]`. Whitespace around each entry is trimmed.
 *
 * @param {string} raw - The raw environment variable value.
 * @returns {string[]} List of non-empty directory paths.
 */
function parseColonList(raw) {
  if (!raw) return [];
  return raw
    .split(':')
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0);
}

/**
 * Resolve the effective list of log directories to scan.
 *
 * Resolution order:
 *   1. `process.env.LOG_DIRS` (colon-separated), if set and non-empty.
 *   2. {@link DEFAULT_LOG_DIRS}.
 *
 * @returns {string[]} Ordered list of directory paths to scan.
 */
function resolveLogDirs() {
  const envDirs = parseColonList(process.env.LOG_DIRS);
  return envDirs.length > 0 ? envDirs : DEFAULT_LOG_DIRS.slice();
}

/**
 * Normalise a single directory path to an absolute path so downstream scanners
 * can rely on stable, unambiguous locations regardless of the process CWD.
 *
 * @param {string} dir - A directory path (absolute or relative).
 * @returns {string} Absolute path.
 */
function toAbsolutePath(dir) {
  return path.resolve(dir);
}

/** Ordered list of directories the backend will scan for logs. */
const LOG_DIRS = resolveLogDirs();

/** Same as {@link LOG_DIRS} but with every entry resolved to an absolute path. */
const LOG_DIRS_ABSOLUTE = LOG_DIRS.map(toAbsolutePath);

module.exports = {
  DEFAULT_LOG_DIRS,
  LOG_DIRS,
  LOG_DIRS_ABSOLUTE,
  parseColonList,
  resolveLogDirs,
  toAbsolutePath,
};
