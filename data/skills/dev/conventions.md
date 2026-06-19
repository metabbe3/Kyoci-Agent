---
name: conventions
description: Coding conventions for Kyoci (typed errors, logging, config, agent behavior rules)
category: software-development
triggers:
  keywords:
    - coding conventions
    - code conventions
    - coding style
    - error handling
    - apperr
    - logging conventions
    - observability
    - how should i write
  regex:
    - "(?i)\\b(apperr|typed error|slog|12-factor)\\b"
priority: normal
---

# Kyoci coding conventions

- **Typed errors:** use `internal/apperr` (`apperr.Newf(key, apperr.KindXxx, msg)`). Wrap with `fmt.Errorf("...: %w", err)`. Never `panic` on expected failures.
- **Logging:** `log/slog` structured (`logger.Info("msg", "key", val)`). Logs go to stdout (12-factor); per-run trace logs are optional job artifacts.
- **Concurrency:** shared structs are goroutine-safe via `sync.RWMutex` (read paths use `RLock`).
- **Config-driven:** add settings to `config/default.yaml` + `KYOCI_*` env overrides in `applyEnvOverrides`. No secrets in YAML.
- **Agents (`agentdef/compose.go`):** every agent prompt ends with "investigate, don't ask", the delegation block, and "Keep responses SHORT. Execute. Verify. Report."
- **OOP + small files:** prefer focused types over god-files; match the surrounding package's style.

When editing, keep changes minimal and idiomatic to the touched package.
