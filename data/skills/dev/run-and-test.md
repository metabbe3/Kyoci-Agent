---
name: run-and-test
description: How to build, run, and test the Kyoci server and web frontend
category: software-development
triggers:
  keywords:
    - go run
    - go test
    - go build
    - npm run dev
    - dev.sh
    - start the server
    - run the server
    - run tests
    - run the tests
  regex:
    - "(?i)\\bgo (run|test|build)\\b"
    - "\\bscripts/dev\\.sh\\b"
priority: normal
---

# Build, run, and test Kyoci

- **Backend:** `go run ./cmd/server` (or `./scripts/dev.sh backend`).
- **Frontend:** `cd web && npm run dev` (Vite; or `./scripts/dev.sh frontend`).
- **Both (foreground):** `./scripts/dev.sh` — logs to `logs/<YYYY-MM-DD>/{backend,frontend}.log`.
- **Tests:** `go test ./...` (targeted: `go test ./internal/orchestrator/...`).
- **Compile check:** `go build ./...`.

Server ports + TLS come from `config/default.yaml` (`server:`); override via `KYOCI_GRPC_PORT`, `KYOCI_REST_PORT`, `KYOCI_AGENT_GRPC_PORT`, `KYOCI_TLS_ENABLED`. Config is loaded once at boot (`config.Load`).
