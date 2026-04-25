---
status: idea
created: "2026-04-25T14:22:00Z"
---

<summary>
- Three small polish changes on `pkg/processor/processor.go` after the typed-primitives refactor lands
- Rename `ready <-chan struct{}` (and `processor.ready` field) to `wakeup` — the channel is a recurring "wake up and rescan" signal, not a one-shot ready notification
- Export `errPreflightSkip` to `ErrPreflightSkip` so external test packages (`pkg/processor_test`) can match it via `errors.Is`
- Reorder `NewProcessor` arguments so all services / interfaces come first, followed by typed config values — a long-standing convention violation
</summary>

<objective>
Final polish pass on `NewProcessor` signature and internal naming after the typed-primitives refactor: rename the wake-up channel, export the preflight-skip sentinel, and group services before configuration.
</objective>

<context>
**Prerequisite:** This prompt depends on `processor-typed-primitives.md` having been applied first. The reorder step assumes the typed parameters (`Dirs`, `Commands`, `ProjectName`, …) are already in place.

Read `CLAUDE.md` for project conventions.

Current shape (`pkg/processor/processor.go`):
- `ready <-chan struct{}` at line 87 — fed by the watcher, consumed in the `Process` event loop at line 214 (`case <-p.ready:`). Recurring signal, not a one-shot.
- `var errPreflightSkip = stderrors.New(...)` at line 57 — sentinel matched by `stderrors.Is(err, errPreflightSkip)` in `processSingleQueued` at line 585. Tests cannot match this from an external test package.
- Constructor argument order intermixes services and configuration values — convention is services / interfaces FIRST, primitive / config types SECOND.
</context>

<requirements>

## 1. Rename `ready` → `wakeup`

In `pkg/processor/processor.go`:
- `NewProcessor` parameter: `ready <-chan struct{}` → `wakeup <-chan struct{}`
- `processor` struct field: `ready` → `wakeup`
- All internal reads in `Process()` (`case <-p.ready:` → `case <-p.wakeup:`)

In `pkg/factory/factory.go` (or wherever `NewProcessor` is called) and the watcher producer:
- Rename the local channel variable consistently
- Rename any `readyCh` / `readyChan` style helper variables

In tests:
- Same rename

The semantic is unchanged: a producer signals on the channel; the daemon loop wakes up and rescans. No behaviour change.

## 2. Export `ErrPreflightSkip`

In `pkg/processor/processor.go`:
- `var errPreflightSkip` → `var ErrPreflightSkip`
- Update the doc comment to start with `ErrPreflightSkip is returned …`
- Update both internal `stderrors.Is(err, errPreflightSkip)` call sites to use the exported name
- Update the `return errPreflightSkip` statement in `checkPreflightConditions`

## 3. Reorder `NewProcessor` arguments

Group services / interfaces FIRST, typed config / values SECOND. Within each group, keep alphabetical-ish or logical-cluster ordering (the exact order inside groups is flexible — the only rule is "services before primitives").

Services / interfaces (group A):
- `executor.Executor`
- `PromptManager`
- `git.Releaser`
- `version.Getter`
- `WorkflowExecutor`
- `spec.AutoCompleter`
- `spec.Lister`
- `notifier.Notifier`
- `executor.ContainerCounter`
- `containerlock.ContainerLock`
- `executor.ContainerChecker`
- `DirtyFileChecker`
- `GitLockChecker`
- `preflight.Checker`
- `validationprompt.Resolver` (added by extract-validationprompt-package.md)
- `wakeup <-chan struct{}` (channel, not a service — but it's runtime wiring; place at the end of group A so the boundary between A and B is clean)

Typed config / values (group B):
- `Dirs`
- `Commands`
- `ProjectName`
- `MaxContainers`
- `AdditionalInstructions`
- `DirtyFileThreshold`
- `AutoRetryLimit`
- `MaxPromptDuration` (`time.Duration`)
- `VerificationGate`

Update:
- `NewProcessor` parameter list
- The body of `NewProcessor` (struct initialization order — purely cosmetic, but keep grouping consistent)
- `processor` struct field order to match the parameter order
- All callers (`pkg/factory/factory.go`, tests, any cmd entrypoint)

## 4. CHANGELOG

Append to `## Unreleased` in `CHANGELOG.md`:

```
- refactor: NewProcessor argument order — services/interfaces first, typed config second; renamed ready→wakeup; exported ErrPreflightSkip for external test packages
```

## 5. Verify

```bash
cd /workspace
make precommit
```

Must exit 0.

</requirements>

<constraints>
- Pure refactor — no behaviour change
- Do NOT add new parameters; do NOT remove existing ones
- All existing tests must pass without re-tuning timing or expectations
- External test packages where applicable
- `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for any new errors (none expected here)
- Do not commit
</constraints>

<verification>
```bash
cd /workspace

# Rename complete
! grep -n "p\.ready\|ready <-chan struct{}\|ready:" pkg/processor/processor.go
grep -n "p\.wakeup\|wakeup <-chan struct{}" pkg/processor/processor.go

# Export complete
! grep -n "errPreflightSkip" pkg/processor/processor.go
grep -n "ErrPreflightSkip" pkg/processor/processor.go

# Constructor order: first interface-typed param appears before first primitive-typed param
# (manual eyeball — automated check is fragile; rely on reviewer reading NewProcessor signature)

make precommit
```
</verification>
