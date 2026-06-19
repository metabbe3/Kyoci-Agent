---
name: install-dependencies
description: How to install dependencies for the Kyoci repo (Go backend + Vite web frontend)
category: software-development
triggers:
  keywords:
    - npm install
    - go mod download
    - go mod tidy
    - pip install
    - yarn install
    - pnpm install
    - install deps
    - install dependencies
    - set up the project locally
    - set up the dev environment
    - setup the repo
  regex:
    - "(?i)\\bgo mod (download|tidy)\\b"
    - "(?i)\\b(cd web|web/) &&? npm install\\b"
priority: normal
---

# Installing dependencies for this repo

Kyoci is a Go backend (`cmd/server`) plus a Vite/React app (`web/`). Go module path: `github.com/metabbe3/Kyoci-Agent`.

- **Go modules:** `go mod download` to fetch; `go mod tidy` after changing imports.
- **Web frontend:** `cd web && npm install` (`web/package.json`; `package-lock.json` is committed — do not regenerate it casually).
- **One-shot:** `./scripts/dev.sh` builds/starts both (see run-and-test).

Never put API keys in `config/default.yaml`. Provide them via env (`KYOCI_PROVIDER_<NAME>_API_KEY`, e.g. `KYOCI_PROVIDER_ANTHROPIC_API_KEY`) or a gitignored file. Verify with `go build ./...`.
