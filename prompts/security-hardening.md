---
status: created
created: "2026-03-08T21:06:35Z"
---

<summary>
- PR titles starting with a dash are rejected before reaching `gh pr create`, preventing argument injection
- Docker NET_ADMIN/NET_RAW capabilities are off by default, opt-in via `netAdmin` config field
- Filesystem watcher errors are logged and recovered instead of causing fatal shutdown
</summary>

<objective>
Harden three security-sensitive areas: CLI argument injection via PR titles, Docker capability escalation defaults, and filesystem watcher resilience.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/git/pr_creator.go` — line ~38 where title is passed to `gh pr create --title`.
Read `pkg/executor/executor.go` — line ~189 where `--cap-add=NET_ADMIN` and `--cap-add=NET_RAW` are hardcoded.
Read `pkg/config/config.go` — config struct to add the new `NetAdmin` field.
Read `pkg/watcher/watcher.go` — lines ~85-91, error from `fsWatcher.Errors` causes immediate return.
Read `pkg/specwatcher/watcher.go` — lines ~79-85, same pattern.
Read `pkg/factory/factory.go` — to wire the new config field into executor.
Read `/home/node/.claude/docs/go-patterns.md`.
</context>

<requirements>
1. In `pkg/git/pr_creator.go`, before passing title to `gh pr create`:
   ```go
   if strings.HasPrefix(title, "-") {
       return "", errors.Errorf(ctx, "invalid PR title: must not start with a dash")
   }
   ```
   Add `"strings"` import if missing.

2. In `pkg/config/config.go`, add to the config struct:
   ```go
   NetAdmin bool `yaml:"netAdmin"`
   ```
   Default should be `false` (capabilities disabled by default).

3. In `pkg/executor/executor.go`:
   a. Add `netAdmin bool` field to the executor struct.
   b. Update the constructor to accept and store this field.
   c. Make the NET_ADMIN/NET_RAW capabilities conditional:
   ```go
   if e.netAdmin {
       args = append(args, "--cap-add=NET_ADMIN", "--cap-add=NET_RAW")
   }
   ```

4. In `pkg/factory/factory.go`, pass `cfg.NetAdmin` to the executor constructor in both call sites:
   - `CreateProcessor` at line ~192 (`NewDockerExecutor` for prompt execution)
   - `CreateSpecGenerator` at line ~136 (`NewDockerExecutor` for spec generation)

5. In `pkg/watcher/watcher.go`, change the error handling in the Watch loop (lines ~85-90):
   ```go
   // Before:
   case err, ok := <-fsWatcher.Errors:
       if !ok {
           return errors.Errorf(ctx, "watcher error channel closed")
       }
       slog.Info("watcher error", "error", err)
       return errors.Wrap(ctx, err, "watcher error")

   // After:
   case err, ok := <-fsWatcher.Errors:
       if !ok {
           return errors.Errorf(ctx, "watcher error channel closed")
       }
       slog.Warn("watcher error", "error", err)
       continue
   ```

6. In `pkg/specwatcher/watcher.go`, apply the same change (lines ~79-84):
   ```go
   // Before:
   case err, ok := <-fsWatcher.Errors:
       if !ok {
           return errors.Errorf(ctx, "watcher error channel closed")
       }
       slog.Info("spec watcher error", "error", err)
       return errors.Wrap(ctx, err, "watcher error")

   // After:
   case err, ok := <-fsWatcher.Errors:
       if !ok {
           return errors.Errorf(ctx, "watcher error channel closed")
       }
       slog.Warn("spec watcher error", "error", err)
       continue
   ```

7. Add tests:
   a. In `pkg/git/pr_creator_test.go` (or create if missing): test that title starting with `--` returns error.
   b. In executor tests: verify `--cap-add` args are present only when `netAdmin=true` and absent when `false`.
</requirements>

<constraints>
- Default `netAdmin` to `false` — breaking change for users who relied on implicit NET_ADMIN, but safer default
- Do NOT change the PRCreator interface
- Do NOT change the Watcher or SpecWatcher interfaces
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Verify PR title validation:
```bash
go test -v ./pkg/git/... -run "PRCreator|CreatePR|title"
```

Verify fsnotify change:
```bash
grep -n "return.*watcher error" pkg/watcher/watcher.go pkg/specwatcher/watcher.go
# Expected: no output (errors are logged, not returned)
```
</verification>

<success_criteria>
- PR titles starting with `-` are rejected with clear error
- Docker NET_ADMIN/NET_RAW capabilities only added when `netAdmin: true` in config
- fsnotify errors logged and continued, not fatal
- All new behavior covered by tests
- `make precommit` passes
</success_criteria>
