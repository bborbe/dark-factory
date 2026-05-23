---
status: idea
tags:
  - dark-factory
  - cli
  - spec
  - idea
---

## Summary

- Native `dark-factory spec generate <id>` CLI subcommand that orchestrates the same claude-yolo container run as the watcher's auto-trigger, but as an explicit host-side action.
- Different ergonomics from `disableAutoGeneratePrompts`: that flag pauses automation; this subcommand replaces it with an explicit invocation.
- Existing `commands/generate-prompts-for-spec.md` is the host-side equivalent today (runs the agent directly, no container); a native subcommand would unify the host- and daemon-triggered paths.
- Breaking change risk if auto-trigger is removed entirely; could ship alongside `disableAutoGeneratePrompts` so operators choose policy vs. explicit-only.

## See also

- `specs/disable-auto-prompt-generation.md` — pause-the-automation alternative; split out from there.
- `commands/generate-prompts-for-spec.md` — current host-side manual path.
