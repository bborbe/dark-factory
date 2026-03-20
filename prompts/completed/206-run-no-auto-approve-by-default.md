---
status: completed
summary: Added --auto-approve flag to dark-factory run; without it generated prompts stay in inbox for manual review instead of being auto-approved and executed
container: dark-factory-206-run-no-auto-approve-by-default
dark-factory-version: v0.59.5-dirty
created: "2026-03-20T14:30:15Z"
queued: "2026-03-20T14:30:15Z"
started: "2026-03-20T14:30:21Z"
completed: "2026-03-20T15:24:57Z"
---

<summary>
- Generated prompts from specs are no longer auto-approved in one-shot mode by default
- New --auto-approve flag opts into the current auto-approve behavior
- Without the flag, generated prompts stay in the inbox for manual review
- Already-queued prompts still execute normally regardless of the flag
- Daemon mode is unaffected (no change)
</summary>

<objective>
Make `dark-factory run` safe by default: generate prompts from approved specs into the inbox, but only execute already-queued prompts. Users who want the current fully-automated behavior opt in with `--auto-approve`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/runner/oneshot.go` — find `generateFromApprovedSpecs` (line 141) and `approveInboxPrompts` (line 194).
Read `main.go` — find the `"run"` case (~line 80) and `ParseArgs` (~line 168).
Read `pkg/factory/factory.go` — find `CreateOneShotRunner` (~line 243).
Read `pkg/config/config.go` — find the `Config` struct (~line 66).
Read `pkg/runner/oneshot_test.go` for existing test patterns.
</context>

<requirements>
1. In `main.go` `ParseArgs`, detect `--auto-approve` flag for the `"run"` command. Current pattern: `-debug` is stripped from rawArgs in a loop. Add the same pattern for `--auto-approve`. Return it as a new 5th return value `autoApprove bool`. Update the `"run"` case in `run()` (~line 80) to pass it to the factory.

2. In `pkg/factory/factory.go` `CreateOneShotRunner`, add `autoApprove bool` as the last parameter. Pass it through to `runner.NewOneShotRunner`.

3. In `pkg/runner/oneshot.go` `NewOneShotRunner`, add `autoApprove bool` as the last parameter after `currentDateTimeGetter`. Store it in the `oneShotRunner` struct as `autoApprove bool`.

4. In `oneShotRunner.generateFromApprovedSpecs`, gate the `approveInboxPrompts` call behind `r.autoApprove`. Change:
```go
moved, err := r.approveInboxPrompts(ctx)
if err != nil {
    return 0, errors.Wrap(ctx, err, "approve inbox prompts")
}
return moved, nil
```
to:
```go
if r.autoApprove {
    moved, err := r.approveInboxPrompts(ctx)
    if err != nil {
        return 0, errors.Wrap(ctx, err, "approve inbox prompts")
    }
    return moved, nil
}

// List generated prompts for manual review
entries, err := os.ReadDir(r.inboxDir)
if err == nil {
    for _, e := range entries {
        if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
            slog.Info("generated prompt awaiting review", "file", e.Name())
        }
    }
}
slog.Info("generated prompts left in inbox — approve with: dark-factory prompt approve <name>")
return 0, nil
```

5. Update `parse_args_test.go` to cover `--auto-approve` flag parsing (both present and absent).

6. Add tests in `pkg/runner/oneshot_test.go`:
   - `autoApprove=true`: generated prompts are moved to in-progress and executed (current behavior preserved)
   - `autoApprove=false` (default): generated prompts remain in inbox directory, only pre-queued prompts execute
</requirements>

<constraints>
- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Follow existing patterns in `pkg/runner/oneshot.go` for field injection
- Do not change daemon mode behavior (`pkg/runner/runner.go`)
- Existing tests must still pass
- Coverage ≥80% for changed code
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
