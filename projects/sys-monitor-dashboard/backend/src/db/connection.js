/**
 * Reusable SQLite connection singleton for the Local-First System Monitor.
 *
 * Design goals
 * ------------
 *  - One shared `better-sqlite3` handle per process (synchronous, fast).
 *  - WAL journal mode for high-throughput log ingestion + concurrent reads.
 *  - Sensible PRAGMAs tuned for a local single-writer workload.
 *  - Graceful close on process exit / signals so the WAL is checkpointed.
 *
 * All data stays strictly local — nothing here ever leaves the machine.
 *
 * NOTE: `package.json` declares `better-sqlite3` as the dependency, so this
 * module is the canonical DB access layer. The legacy `schema.js` exports a
 * `SCHEMA_SQL` constant we reuse here so the two never drift.
 */

import Database from 'better-sqlite3';
import { SCHEMA_SQL } from './schema.js';

/** Default location for the SQLite file (overridable via env for tests). */
const DEFAULT_DB_PATH =
  process.env.SQLITE_DB_PATH ||
  new URL('../../data/sys-monitor.sqlite', import.meta.url).pathname;

/**
 * Tuned PRAGMAs for a local-first, single-writer log database.
 * - `journal_mode = WAL`  : readers never block the writer; great for dashboards.
 * - `synchronous = NORMAL`: safe under WAL, ~2-5x faster than FULL.
 * - `foreign_keys = ON`   : future-proofing for related tables.
 * - `busy_timeout`        : wait rather than throw on rare lock contention.
 */
const PRAGMAS = [
  'journal_mode = WAL',
  'synchronous = NORMAL',
  'foreign_keys = ON',
  'busy_timeout = 5000',
  'temp_store = MEMORY',
];

let _instance = null; // cached singleton Database handle
let _closeHandlersBound = false;

/**
 * Apply the canonical schema (idempotent) to an open database handle.
 *
 * @param {Database} db - An open better-sqlite3 Database.
 * @returns {Database} The same handle, for chaining.
 */
function applySchema(db) {
  db.exec(SCHEMA_SQL);
  return db;
}

/**
 * Apply the tuned PRAGMA settings to an open database handle.
 *
 * @param {Database} db - An open better-sqlite3 Database.
 * @returns {Database} The same handle, for chaining.
 */
function applyPragmas(db) {
  for (const pragma of PRAGMAS) {
    // `db.pragma` with a value sets it; we ignore the returned row.
    db.pragma(pragma);
  }
  return db;
}

/**
 * Register process-level handlers that close the DB gracefully.
 *
 * Bound exactly once per process; idempotent via `_closeHandlersBound`.
 * Catches the rare "close throws" case so a signal never crashes the app.
 *
 * @param {Database} db - The handle to close on exit.
 */
function bindGracefulClose(db) {
  if (_closeHandlersBound) return;
  _closeHandlersBound = true;

  const closeOnce = () => {
    try {
      if (db.open) db.close();
    } catch {
      /* swallow — best-effort cleanup during shutdown */
    }
  };

  for (const signal of ['exit', 'SIGINT', 'SIGTERM', 'SIGHUP']) {
    process.on(signal, closeOnce);
  }
}

/**
 * Open (or return the cached) shared SQLite database handle.
 *
 * The first call opens the file at `dbPath` (or `DEFAULT_DB_PATH`), applies
 * the PRAGMAs and schema, and wires up graceful-close handlers. Subsequent
 * calls return the same handle, ignoring `dbPath`.
 *
 * @param {string} [dbPath] - Optional override path (mainly for tests).
 * @returns {Database} A ready-to-use better-sqlite3 Database handle.
 */
export function getDb(dbPath = DEFAULT_DB_PATH) {
  if (_instance) return _instance;

  const db = new Database(dbPath);
  applyPragmas(db);
  applySchema(db);
  bindGracefulClose(db);

  _instance = db;
  return _instance;
}

/**
 * Close the shared handle if one exists and clear the singleton.
 *
 * Intended for tests / explicit teardown. After calling this, `getDb()`
 * will open a fresh connection on its next invocation.
 *
 * @returns {boolean} `true` if a handle was closed, `false` otherwise.
 */
export function closeDb() {
  if (!_instance) return false;
  try {
    if (_instance.open) _instance.close();
  } finally {
    _instance = null;
  }
  return true;
}

/** Exported for tests / diagnostics. */
export const __internals = { DEFAULT_DB_PATH, PRAGMAS };

export default getDb;
