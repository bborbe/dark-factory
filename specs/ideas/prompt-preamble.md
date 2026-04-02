---
status: idea
tags:
  - dark-factory
  - spec
---

## Summary

- Add `promptPreamble` config field to `.dark-factory.yaml` — a text block prepended to every prompt
- Injected into both prompt execution and spec-to-prompt generation
- Allows telling the agent about shared docs, conventions, or project-specific instructions

## Problem

With `extraMounts`, users can mount shared docs into the container. But the agent doesn't know they exist unless each prompt explicitly references them. Users must add "read /docs/..." to every prompt's `<context>` section manually. This is repetitive and easy to forget.

More broadly, there's no way to give the agent project-wide instructions that apply to all prompts without editing CLAUDE.md in the claude-yolo config (which affects all projects, not just one).

## Goal

After this work, users configure a `promptPreamble` in `.dark-factory.yaml` that is automatically prepended to every prompt. The agent sees it as part of the prompt content — no per-prompt boilerplate needed.

## Example

```yaml
extraMounts:
  - src: ../docs/howto
    dst: /docs

promptPreamble: |
  Also read shared documentation at /docs for coding guidelines and patterns.
  Follow the conventions in /docs/go-testing-guide.md for all test code.
```

Every prompt execution and spec generation gets this preamble prepended.

## Do-Nothing Option

Users add doc references to every prompt manually, or put instructions in CLAUDE.md (which applies globally to all projects). Neither is per-project or automatic.
