---
name: codebase-structure
description: How the Kyoci-Agent codebase is organized (where routing, agents, skills, tools live)
category: software-development
triggers:
  keywords:
    - codebase structure
    - project structure
    - how is the code organized
    - where is the code
    - where is the project
    - codebase layout
    - repo layout
    - navigate the codebase
  regex:
    - "(?i)\\b(where|how).*(code|codebase|project|repo).*\\b"
priority: normal
---

# Kyoci-Agent layout

Go backend (`github.com/metabbe3/Kyoci-Agent`) + Vite frontend (`web/`).

- `cmd/server/main.go` — process entrypoint.
- `internal/orchestrator/` — task routing + `Execute` lifecycle: `orchestrator.go`, `classifier.go` + `autoresolve.go` (deterministic routing + cache), `delegation.go`, `hitl.go`, `intelligence.go`, `workspace.go`, `runlog.go`.
- `internal/agent/` — execution loop: `loop.go` (ReAct), `orchestrated.go`/`worker.go` (planner→workers→synth), `prompts.go`, `explore_worker.go`, `ports.go`.
- `internal/agentdef/` — markdown agent definitions + the scorer: `def.go`, `loader.go`, `match.go`, `compose.go`.
- `internal/role/` — `RoleRegistry` (`registry.go`) binds tools/skills/model to each agent.
- `internal/skill/` + `internal/skill/builtin/` — zero-AI deterministic skills.
- `internal/promptskill/` — per-task procedural-knowledge injection (loaded from `data/skills/`).
- `internal/tool/` + `internal/tool/builtin/` — tools (file, grep, terminal, delegation, …).
- `internal/llm/` — providers + router; `internal/config/`, `internal/memory/`, `internal/apperr/` (typed errors), `internal/taskctx/`, `internal/tracing/`.
- `pkg/` — shared types (`role.go`, `skill.go`, `tool.go`, `provider.go`, `types.go`).
- `agents/*.md` — the agent definitions (developer, frontend, qa, sre, pm, generalist).
- `config/default.yaml` — config. `data/skills/` — prompt-skills.

Routing = `agentdef.BestMatch` (keyword/anchor/regex) wrapped by `resolveAgent` (cache + provenance); generalist is the fallback and re-delegates via `delegation.go`.
