---
name: efficient-code-search
description: How to search/read/analyze/edit code by pattern (not whole files) to conserve context
category: software-development
triggers:
  keywords:
    - search the code
    - search the codebase
    - find in the code
    - find in code
    - grep for
    - find all references
    - find all uses
    - analyze the code
    - read the code
    - where is the code
    - code search
    - whole file
    - entire file
priority: normal
---

# Search and edit code by pattern — keep context lean

Never dump whole files into your context or your response. Small models drift when the prompt is bloated.

- **Locate by pattern first:** use `grep` (content), `glob` (paths), or `codesearch` to find where something lives — search for the symbol/pattern, not by opening files blindly.
- **Read targeted regions:** open only the relevant lines of a file (targeted / offset read), never the whole file.
- **Cite `file:line`** for every claim so the caller can verify.
- **Edit precisely:** use `patch` for targeted changes; do not rewrite whole files.
- **Prefer zero-AI skills** for deterministic ops (json/yaml/csv formatting, hashing, base64, regex, color, uuid, subnet, cron, etc.) instead of computing them yourself — they are instant and exact.
- Report findings in a few lines; quote only the exact lines that matter.
