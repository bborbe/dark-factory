---
status: idea
tags:
  - dark-factory
  - telemetry
  - spec
  - idea
---

## Summary

- Surface which specs are currently sitting at `status: approved` in `specs/in-progress/` with auto-generation disabled — i.e. specs waiting on a manual `/dark-factory:generate-prompts-for-spec` invocation.
- Candidate surfaces: `dark-factory status` CLI output, a daemon HTTP endpoint, or a `slog` info-level summary at the daemon's existing idle-log interval.
- Depends on `autoGeneratePrompts` being shipped and in use (with operators opting in via `autoGeneratePrompts: true`) so there's a real "awaiting" state to surface.

## See also

- `specs/disable-auto-prompt-generation.md` — flag that creates the "awaiting" state; split out from there.
