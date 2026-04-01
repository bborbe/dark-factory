---
status: idea
---

## Summary

- dark-factory supports running from a subdirectory of a git repo (monorepo)
- The YOLO container mounts the git root (not the `.dark-factory.yaml` directory)
- Specs, prompts, scenarios, and CLAUDE.md are relative to the `.dark-factory.yaml` location
- Multiple independent dark-factory instances can run in the same repo (one per subdirectory)
- No new config field needed — git root is auto-detected via `git rev-parse --show-toplevel`

## Problem

In monorepos (e.g., a trading repo with 50+ services), a single `specs/` and `prompts/` directory at the repo root mixes unrelated topics. Services share libraries, so the container must see the full repo. Currently dark-factory mounts the directory where `.dark-factory.yaml` lives — if that's a subdirectory, shared code is invisible to the container.

## Goal

A user can place `.dark-factory.yaml` in any subdirectory of a git repo. The container sees the full repo (mounted at git root). Specs, prompts, logs, and CLAUDE.md are scoped to that subdirectory. Multiple subdirectories can each have their own dark-factory instance running independently.

## Do-Nothing Option

Users must keep one `.dark-factory.yaml` at the repo root, resulting in a single mixed queue of specs/prompts across all services. This doesn't scale for large monorepos.

## Desired Behavior

1. dark-factory detects git root via `git rev-parse --show-toplevel`
2. Container mount root = git root (not `.dark-factory.yaml` directory)
3. Container working directory = subdirectory where `.dark-factory.yaml` lives (relative to git root)
4. `make precommit` runs from the subdirectory (not git root)
5. Each subdirectory has its own lock file, queue, and daemon
6. Multiple daemons in the same repo do not conflict (separate locks)
7. Git operations (fetch, branch, commit, push) work from git root as before
8. Existing single-root projects (`.dark-factory.yaml` at git root) behave identically — no breaking change
