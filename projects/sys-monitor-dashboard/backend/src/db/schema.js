/**
 * SQLite schema definition for the Local-First System Monitor & Log Analyzer.
 *
 * All data stays strictly local — this module only defines structure and
 * initialization helpers; it never transmits data off the machine.
 */

const sqlite3 = require('sqlite3').verbose();

/**
 * Canonical SQL DDL for the parsed-logs database.
 *
 * Tables:
 *   - logs: one row per parsed log line.
 *
 * Indexes:
 *   - idx_logs_timestamp : speeds up time-range queries and ordering.
 *   - idx_logs_level     : speeds up filtering by severity (INFO/WARN/ERROR).
 *   - idx_logs_level_timestamp : composite index for the very common
 *     "level within a time window" query.
 */
const SCHEMA_SQL = `
CREATE TABLE IF NOT EXISTS logs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp     TEXT    NOT NULL,          -- ISO-8601 (UTC) for portable ordering
    level         TEXT    NOT NULL,          -- INFO | WARN | ERROR (normalized)
    message       TEXT    NOT NULL,          -- extracted human-readable message
    source_file   TEXT    NOT NULL,          -- absolute path of the originating log file
    raw_line      TEXT    NOT NULL,          -- original unparsed line, kept for audit
    created_at    TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_logs_timestamp
    ON logs (timestamp);

CREATE INDEX IF NOT EXISTS idx_logs_level
    ON logs (level);

CREATE INDEX IF NOT EXISTS idx_logs_level_timestamp
    ON logs (level, timestamp);
`;

/**
 * The set of log levels the system recognizes and normalizes to.
 * Kept here so both the parser and the schema agree on the vocabulary.
 */
const SUPPORTED_LEVELS = Object.freeze(['INFO', 'WARN', 'ERROR']);

/**
 * Apply the schema to an open sqlite3 database handle.
 *
 * @param {sqlite3.Database} db - An open sqlite3 Database instance.
 * @returns {Promise<void>} Resolves once all DDL statements have executed.
 */
function applySchema(db) {
  return new Promise((resolve, reject) => {
    db.exec(SCHEMA_SQL, (err) => {
      if (err) {
        reject(new Error(`Failed to apply schema: ${err.message}`));
        return;
      }
      resolve();
    });
  });
}

/**
 * Open (or create) a SQLite database file and ensure the schema exists.
 *
 * @param {string} dbPath - Absolute path to the .sqlite/.db file.
 * @returns {Promise<sqlite3.Database>} The initialized database handle.
 */
function initDatabase(dbPath) {
  return new Promise((resolve, reject) => {
    const db = new sqlite3.Database(dbPath, (openErr) => {
      if (openErr) {
        reject(new Error(`Cannot open database at ${dbPath}: ${openErr.message}`));
        return;
      }
      applySchema(db).then(() => resolve(db)).catch(reject);
    });
  });
}

module.exports = {
  SCHEMA_SQL,
  SUPPORTED_LEVELS,
  applySchema,
  initDatabase,
};
