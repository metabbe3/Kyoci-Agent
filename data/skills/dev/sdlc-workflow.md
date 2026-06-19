---
name: sdlc-workflow
description: Good-developer SDLC discipline for code tasks — setup deps first, write modular/OOP/DRY code, verify by building
category: software-development
triggers:
  keywords:
    - set up a project
    - write a program
    - write an app
    - scaffold a project
    - build an app
    - build a website
    - create a component
  regex:
    - "(?i)\\b(build|create|make|implement|scaffold|develop)\\b.*\\b(app|application|program|project|service|cli|website|component|module)\\b"
priority: normal
---

# SDLC discipline — work like a good developer

For any code-creation task, follow this order:

1. **SETUP first.** Create the project manifest (`package.json` / `go.mod` / `requirements.txt`) and install dependencies (`npm install`, `go mod download`, `pip install`) BEFORE writing feature code. A step that writes source before deps exist will fail to build.
2. **IMPLEMENT as small, reusable modules.** One responsibility per file. Extract repeated logic into named functions (DRY). Prefer composition / OOP (small types/interfaces) over monolithic files. Reuse what you already wrote instead of duplicating it.
3. **VERIFY last.** Run the actual build/tests (`npm run build`, `go build ./...`, `go test ./...`) via the terminal and report the REAL pass/fail output. Never claim success without running it.

If a command fails, read its output, fix the cause, and retry — but do not repeat the exact same failing command more than twice (the runtime will circuit-break loops).
