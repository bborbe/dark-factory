---
status: idea
tags:
  - dark-factory
  - spec
  - config
  - idea
---

## Summary

- Per-spec frontmatter override `disableAutoGenerate: true` on individual spec files, opting out of auto-generation for one spec while leaving the global / project flag at its default.
- Zero-value (key absent) preserves the global flag's behavior.
- Touches `pkg/spec/spec.go` `Frontmatter` struct, the watcher's gate check (which today only consults `Config.DisableAutoGeneratePrompts`), and any spec-creation tooling that writes frontmatter.

## See also

- `specs/disable-auto-prompt-generation.md` — global flag this builds on; split out from there as a separate "why" (per-spec authoring control vs. operator-wide policy).
