---
status: draft
tags:
  - dark-factory
  - spec
---

## Summary

- Add optional `maxContainers` field to per-project `.dark-factory.yaml`
- Overrides the global `~/.dark-factory/config.yaml` limit for this project
- Lower values restrict the project to fewer slots (low-priority repos)
- Higher values allow priority projects to run more containers than the global default
- Missing field falls back to global limit

## Problem

The global `maxContainers` limit applies uniformly to all projects. When running 40 repos, some are high-priority (trading strategies, active features) and some are low-priority (dependency updates, maintenance). There's no way to say "this project can use 5 slots" or "this project should never use more than 1 slot."

## Goal

After this work, per-project `.dark-factory.yaml` can override the global container limit. Priority projects get more slots, background projects get fewer. The global limit remains the default when no per-project value is set.

## Non-goals

- Cross-project fairness scheduling (no global arbiter that balances slots between projects)
- Dynamic limit adjustment based on system load
- Per-prompt container limits (this is per-project only)

## Assumptions

- The existing global `maxContainers` counting mechanism (docker ps with labels) is reused
- Each daemon independently enforces its own limit — no coordination between daemons
- Two priority projects both set to 5 can exceed the combined system capacity — this is acceptable (same race tolerance as global limit)

## Desired Behavior

1. **Config field**: `.dark-factory.yaml` supports an optional `maxContainers` integer field. When present, it overrides the global limit for this project's daemon.

   ```yaml
   # Priority project
   maxContainers: 5

   # Background project
   maxContainers: 1
   ```

2. **Fallback to global**: When `maxContainers` is missing or zero in `.dark-factory.yaml`, the daemon uses the global limit from `~/.dark-factory/config.yaml` (default: 3).

3. **Wait loop uses project limit**: Before starting a container, the daemon compares the system-wide running container count against this project's `maxContainers` value instead of always using the global value.

4. **Status display**: `dark-factory status` shows the effective limit (project or global) in the `Containers: N/M` line.

## Constraints

- Existing global `maxContainers` in `~/.dark-factory/config.yaml` continues to work as before
- No inter-process coordination — each daemon reads its own config independently
- Validation: `maxContainers` must be >= 1 if present (same as global validation)
- No changes to Docker label scheme or container counting mechanism
- All existing tests must pass, `make precommit` passes

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `maxContainers` missing from project config | Falls back to global limit | N/A |
| `maxContainers: 0` in project config | Treated as unset, falls back to global | N/A |
| `maxContainers: -1` in project config | Validation error at startup | User fixes config |
| Multiple projects with high limits run simultaneously | Combined containers exceed any single limit | Acceptable — soft limit, self-corrects |
| Global config missing AND project config missing | Default 3 (existing behavior) | N/A |

## Security / Abuse Cases

- Config is user-owned and trusted — no new attack surface
- A high per-project limit doesn't grant more API quota or bypass rate limits

## Acceptance Criteria

- [ ] `maxContainers` field parsed from per-project `.dark-factory.yaml`
- [ ] Missing or zero value falls back to global limit
- [ ] Daemon wait loop uses the project-level limit when set
- [ ] `dark-factory status` shows effective limit
- [ ] Validation rejects negative values
- [ ] `docs/configuration.md` updated with per-project `maxContainers` documentation
- [ ] All existing tests pass, `make precommit` passes

## Verification

```bash
make precommit
```

Manual verification:

1. Set `maxContainers: 1` in project config, approve 3 prompts — only 1 runs at a time
2. Set `maxContainers: 5` in project config, global is 3 — project allows up to 5
3. Remove `maxContainers` from project config — falls back to global 3
4. `dark-factory status` shows correct effective limit

## Do-Nothing Option

All projects share the global limit equally. A low-priority dependency update project can starve a high-priority feature project. Users must manually stop/start daemons to prioritize.
