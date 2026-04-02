---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-04-02T07:58:51Z"
prompted: "2026-04-02T08:01:31Z"
verifying: "2026-04-02T09:05:28Z"
branch: dark-factory/extra-mounts
---

## Summary

- Add `extraMounts` config field to `.dark-factory.yaml` for additional Docker volume mounts in YOLO containers
- Mounts are read-only by default to prevent agents from modifying shared resources
- Allows sharing docs, guides, or config across repos without duplicating files

## Problem

Dark-factory mounts only the project workspace, claude config dir, and a few optional files (netrc, gitconfig) into the container. Shared documentation (e.g., migration guides in a parent monorepo, coding guidelines) must be copied into each repo's `docs/` directory. This causes:
1. **Duplication** across repos
2. **Divergence** when agents update their local copy
3. **Manual effort** to sync changes back

Real-world case: sm-octopus has 17 repos sharing a tooling update guide. Currently copied into each repo.

## Goal

After this work, users can configure additional volume mounts in `.dark-factory.yaml`. The mounts are injected as `-v` flags on the `docker run` command, following the same pattern as existing netrc/gitconfig mounts.

## Non-goals

- Writable extra mounts (read-only is the safe default; writable can be added later if needed)
- Validation that destination paths don't conflict with existing mounts
- Mount propagation or bind-mount options beyond `ro`

## Desired Behavior

1. **Config field**: `.dark-factory.yaml` supports an optional `extraMounts` list. Each entry has `src` (host path), `dst` (container path), and optional `readonly` (default: `true`).

   ```yaml
   extraMounts:
     - src: ../docs/howto
       dst: /docs
     - src: ~/Documents/workspaces/coding/docs
       dst: /coding-docs
       readonly: true
   ```

2. **Path resolution**: `src` paths are resolved relative to the project root directory. Tilde (`~/`) is expanded to the user's home directory. Absolute paths are used as-is.

3. **Docker flag injection**: Each extra mount is added as a volume mount to the YOLO container, following the same pattern as netrc and gitconfig mounts. Read-only mounts use `:ro` suffix; if `readonly: false`, the mount is writable.

4. **Config validation**: At config load time, verify `src` is not empty and `dst` is not empty.

5. **Runtime validation**: At execution time, verify each `src` exists on the filesystem. If missing, log a warning and skip that mount (not fatal).

## Constraints

- Follow the same mount pattern used for netrc/gitconfig
- `readonly` defaults to `true` — safety first
- No changes to existing mounts or their ordering
- All existing tests must pass, `make precommit` passes

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `src` path does not exist | Log warning, skip that mount, continue | User fixes path |
| `dst` conflicts with existing mount (e.g., `/workspace`) | Log warning, mount still added (Docker last-mount-wins) | User fixes config |
| `extraMounts` field missing | No extra mounts, existing behavior unchanged | N/A |
| Relative `src` resolves outside project tree | Allowed — user is trusted (same as netrc/gitconfig) | N/A |

## Security / Abuse Cases

- Extra mounts are read-only by default — agent cannot modify shared resources
- Paths come from `.dark-factory.yaml` which is user-owned and trusted (same trust model as other config fields)
- No new attack surface beyond what netrc/gitconfig mounts already expose

## Acceptance Criteria

- [ ] `extraMounts` config field parsed from `.dark-factory.yaml`
- [ ] Each extra mount becomes a `-v` flag in `docker run`
- [ ] `readonly` defaults to `true` (`:ro` suffix)
- [ ] Relative `src` resolved from project root
- [ ] Tilde in `src` expanded to home directory
- [ ] Missing `src` logged as warning and skipped (not fatal)
- [ ] Empty `extraMounts` or missing field = no change to existing behavior
- [ ] `docs/configuration.md` updated with `extraMounts` field documentation
- [ ] All existing tests pass, `make precommit` passes

## Verification

```bash
make precommit
```

Manual verification:

1. Add `extraMounts` with a valid `src` dir to `.dark-factory.yaml`
2. Run a prompt, verify the mount appears in container (`ls /docs`)
3. Verify mount is read-only (write attempt fails)
4. Remove `extraMounts` field, verify existing behavior unchanged

## Do-Nothing Option

Users copy shared documentation into each repo. With 17 repos, that is 17 copies to maintain. Agents may modify their local copy, causing divergence. Acceptable for 2-3 repos, unmanageable at scale.
