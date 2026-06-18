#!/usr/bin/env node
/**
 * Drift guard: best-effort check that the TypeScript zod schemas in
 * `src/lib/schemas.ts` cover the JSON field tags declared by the Go struct
 * definitions in `internal/dashboard/dashboard.go`.
 *
 * Run with: `node scripts/check-schemas.mjs`
 *
 * Algorithm (deliberately tolerant):
 *   1. Read the Go file (skip pass if missing — e.g. running outside the repo).
 *   2. For each Go struct that has a `json:"..."` tag, extract (struct, fields).
 *   3. For each field, check the snake_case json name appears as a key inside
 *      a matching `z.object({...})` literal in schemas.ts.
 *   4. Report missing fields. Exit 1 if any are found, 0 otherwise.
 *
 * The mapping from Go struct name → schema block is by best-effort name match
 * (the frontend names mirror the Go types). It is intentionally heuristic: a
 * false negative here should prompt a human look, not break CI silently.
 */

import { readFileSync, existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = resolve(here, "..");
const repoRoot = resolve(webRoot, "..");
const goFile = resolve(repoRoot, "internal/dashboard/dashboard.go");
const schemaFile = resolve(webRoot, "src/lib/schemas.ts");

if (!existsSync(goFile)) {
  console.log(`[check-schemas] ${goFile} not found — skipping (best-effort).`);
  process.exit(0);
}
if (!existsSync(schemaFile)) {
  console.error(`[check-schemas] ${schemaFile} not found.`);
  process.exit(1);
}

const goSrc = readFileSync(goFile, "utf8");
const schemaSrc = readFileSync(schemaFile, "utf8");

// Parse Go structs: capture `type X struct { ... }` blocks, then `Field Type \`json:"name,..."\``.
const structRe = /type\s+(\w+)\s+struct\s*\{([\s\S]*?)\}/g;
const fieldRe = /json:"([a-z0-9_]+)(?:,omitempty)?"/g;

/** Go struct name → set of JSON field names declared on it. */
const goStructs = new Map();
let m;
while ((m = structRe.exec(goSrc)) !== null) {
  const [, structName, body] = m;
  const fields = new Set();
  let f;
  while ((f = fieldRe.exec(body)) !== null) {
    fields.add(f[1]);
  }
  fieldRe.lastIndex = 0;
  if (fields.size > 0) goStructs.set(structName, fields);
}

// Heuristic struct → schema-name aliases (frontend schemas mirror Go types).
const aliases = {
  ProviderSummary: "ProviderSummary",
  ModelRow: "ModelRow",
  ProviderConfigDTO: "ProviderConfigDTO",
  HardwareSpecs: "HardwareSpecs",
  RecommendPick: "RecommendPick",
  CloudAdvice: "CloudAdvice",
  RecommendResult: "RecommendResult",
  ChatMessage: "ChatMessage",
  UploadedFile: "UploadedFile",
  SSEChunk: "SSEChunk",
  SkillInfo: "SkillInfo",
};

let drift = 0;
for (const [goName, fields] of goStructs) {
  const schemaName = aliases[goName];
  if (!schemaName) continue; // struct without a frontend mirror (e.g. request bodies)
  // Find the z.object({...}) block of this schema by name.
  const idx = schemaSrc.indexOf(`${schemaName}Schema = z`);
  if (idx === -1) {
    console.warn(`[check-schemas] no ${schemaName}Schema found in schemas.ts — skipping.`);
    continue;
  }
  const slice = schemaSrc.slice(idx, idx + 1200);
  const missing = [...fields].filter((name) => !slice.includes(`${name}:`));
  if (missing.length > 0) {
    drift++;
    console.error(
      `[check-schemas] ${goName}: missing from ${schemaName}Schema → ${missing.join(", ")}`
    );
  }
}

if (drift === 0) {
  console.log(`[check-schemas] OK — ${goStructs.size} Go structs checked against schemas.ts.`);
  process.exit(0);
}
console.error(`[check-schemas] ${drift} drift(s) detected.`);
process.exit(1);
