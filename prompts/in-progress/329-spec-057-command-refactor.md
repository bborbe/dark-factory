---
status: approved
spec: [057-lifecycle-state-machine]
created: "2026-04-25T09:27:35Z"
queued: "2026-04-25T09:35:25Z"
---

<summary>
- Every transition gate in the six target commands (`spec approve/unapprove/complete`, `prompt approve/unapprove/complete`) in `pkg/cmd/` is replaced with a `CanTransitionTo()` call — the boolean check is delegated to the model, not hard-coded in the command
- The one literal string comparison in `pkg/cmd/` (`spec_list.go`'s `e.Status == "verifying"`) is replaced with a typed constant comparison
- For cases that errored before the refactor, user-visible error messages are byte-identical (asserted by existing tests). Cases that were previously silent no-ops (e.g. `spec approve` from `prompted`) now error via `CanTransitionTo()` — this tightening is intentional and matches the spec.
- `! grep -rn 'Status == "' pkg/cmd/` and `! grep -rn 'status != "' pkg/cmd/` both succeed (zero matches)
- Adding a single row to `specTransitions` or `promptTransitions` is the only change needed to allow a new transition — no command file change required
- All existing tests pass unchanged
</summary>

<objective>
Replace ad-hoc status string comparisons in `pkg/cmd/` with calls to the `CanTransitionTo()` method added in prompt 1. After this prompt, the transition logic lives entirely in the model layer and all verification greps pass.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

**Prerequisite**: This prompt depends on prompt 1 (`1-spec-057-lifecycle-model.md`) having been applied first. The `CanTransitionTo()` method, predicates, and `AvailableSpecStatuses` must already exist in `pkg/spec/spec.go` and `pkg/prompt/prompt.go`.

Key files to read before editing:
- `pkg/spec/spec.go` — `Status.CanTransitionTo()`, `Status.IsActive()`, `Status.IsPreExecution()`, `StatusVerifying` constant
- `pkg/prompt/prompt.go` — `PromptStatus.CanTransitionTo()`, `PromptStatus.IsActive()`
- `pkg/cmd/spec_approve.go` — line 66: `sf.Frontmatter.Status == string(spec.StatusApproved)`
- `pkg/cmd/spec_unapprove.go` — line 71: `sf.Frontmatter.Status != string(spec.StatusApproved)`
- `pkg/cmd/spec_complete.go` — line 68: `sf.Frontmatter.Status != string(spec.StatusVerifying)`
- `pkg/cmd/unapprove.go` — line 74: `pf.Frontmatter.Status != string(prompt.ApprovedPromptStatus)`
- `pkg/cmd/spec_list.go` — line 92: `e.Status == "verifying"` (literal string — the grep violation)
- `pkg/cmd/spec_approve_test.go`, `pkg/cmd/spec_unapprove_test.go`, `pkg/cmd/spec_complete_test.go`, `pkg/cmd/approve_test.go`, `pkg/cmd/unapprove_test.go` — read these to understand what error messages the existing tests assert; you MUST preserve those exact strings
</context>

<requirements>

## 1. `pkg/cmd/spec_approve.go` — replace literal equality check

Current code (line ~66):
```go
if sf.Frontmatter.Status == string(spec.StatusApproved) {
    return errors.Errorf(ctx, "spec is already approved")
}
```

Replace with:
```go
if err := spec.Status(sf.Frontmatter.Status).CanTransitionTo(spec.StatusApproved); err != nil {
    return errors.Errorf(ctx, "spec is already approved")
}
```

The boolean test is now delegated to `CanTransitionTo`; the user-facing message is preserved byte-identical. Note: the current check fires when status IS approved (can't re-approve). With `CanTransitionTo`, it fires whenever the transition is invalid — which includes `approved → approved` (not in the table) as well as `completed → approved`, etc. This is strictly more correct.

Read `pkg/cmd/spec_approve_test.go` to confirm the test still expects "spec is already approved" — preserve that string exactly.

## 2. `pkg/cmd/spec_unapprove.go` — replace inequality check

Current code (line ~71):
```go
if sf.Frontmatter.Status != string(spec.StatusApproved) {
    return errors.Errorf(
        ctx,
        "cannot unapprove spec with status %q: only approved specs can be unapproved",
        sf.Frontmatter.Status,
    )
}
```

The target transition for "unapprove" is `approved → draft`. Replace with a `CanTransitionTo(spec.StatusDraft)` check:

```go
if err := spec.Status(sf.Frontmatter.Status).CanTransitionTo(spec.StatusDraft); err != nil {
    return errors.Errorf(
        ctx,
        "cannot unapprove spec with status %q: only approved specs can be unapproved",
        sf.Frontmatter.Status,
    )
}
```

Read `pkg/cmd/spec_unapprove_test.go` to confirm the preserved message matches.

## 3. `pkg/cmd/spec_complete.go` — replace inequality check

Current code (line ~68):
```go
if sf.Frontmatter.Status != string(spec.StatusVerifying) {
    return errors.Errorf(
        ctx,
        "spec is not in verifying state (current: %s)",
        sf.Frontmatter.Status,
    )
}
```

The transition is `verifying → completed`. Replace with:

```go
if err := spec.Status(sf.Frontmatter.Status).CanTransitionTo(spec.StatusCompleted); err != nil {
    return errors.Errorf(
        ctx,
        "spec is not in verifying state (current: %s)",
        sf.Frontmatter.Status,
    )
}
```

Read `pkg/cmd/spec_complete_test.go` to confirm the preserved message matches.

## 4. `pkg/cmd/unapprove.go` — replace inequality check on prompt

Current code (line ~74):
```go
if pf.Frontmatter.Status != string(prompt.ApprovedPromptStatus) {
    return errors.Errorf(
        ctx,
        "cannot unapprove prompt with status %q: only approved prompts can be unapproved",
        pf.Frontmatter.Status,
    )
}
```

The transition is `approved → draft`. Replace with:

```go
if err := prompt.PromptStatus(pf.Frontmatter.Status).CanTransitionTo(prompt.DraftPromptStatus); err != nil {
    return errors.Errorf(
        ctx,
        "cannot unapprove prompt with status %q: only approved prompts can be unapproved",
        pf.Frontmatter.Status,
    )
}
```

Read `pkg/cmd/unapprove_test.go` to confirm the preserved message matches.

## 5. `pkg/cmd/spec_list.go` — fix literal string comparison

Current code (line ~92):
```go
if e.Status == "verifying" {
    status = "!" + e.Status
}
```

`e.Status` is a `string` field of `SpecEntry`. Replace the literal string with the typed constant:

```go
if e.Status == spec.StatusVerifying.String() {
    status = "!" + e.Status
}
```

Or equivalently (and more idiomatically):
```go
if spec.Status(e.Status) == spec.StatusVerifying {
    status = "!" + e.Status
}
```

Use whichever form passes `make precommit`. Add import for `"github.com/bborbe/dark-factory/pkg/spec"` to `spec_list.go` if not already present. Read the file first to check.

## 6. `pkg/cmd/prompt_complete.go` — audit for compliance

Read `pkg/cmd/prompt_complete.go`. The current switch statement uses typed constants (`prompt.PendingVerificationPromptStatus` etc.) without literal strings, so it does NOT violate the grep checks. Do NOT modify it unless the grep check fails.

After all other changes, run the grep checks (step 9). If `prompt_complete.go` produces a hit, replace the switch with a predicate check. If it does not produce a hit, leave it unchanged.

## 7. Run `make test` after changes

```bash
cd /workspace && make test
```

All tests must pass. If any test fails due to an error message mismatch, read the test file to find the expected string and adjust ONLY the error message in the command file to match — do not modify the test.

## 8. Write CHANGELOG entry

If `## Unreleased` already exists in `CHANGELOG.md` (from prompt 1), append to it. Otherwise add it. Append:

```
- refactor: replace ad-hoc status string comparisons in pkg/cmd/ with CanTransitionTo() and typed constant checks
```

## 9. Final verification greps

Run both of these — they must both succeed (exit 0, zero output):

```bash
! grep -rn 'Status == "' pkg/cmd/
! grep -rn 'status != "' pkg/cmd/
```

If either grep produces output, fix the remaining literal string comparison before proceeding to `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- All existing tests in `pkg/cmd/`, `pkg/spec/`, `pkg/prompt/`, `pkg/specwatcher/`, `pkg/processor/` must pass unchanged — do NOT modify any test file
- For cases that errored before the refactor, user-visible error messages must be byte-identical (these are the cases asserted by existing tests, which must continue to pass unchanged). For cases that were silently no-op before but error now via `CanTransitionTo()`, the message stays the original wording — this is the intentional tightening described in the spec
- `prompt_complete.go` switch statement is already grep-compliant (uses typed constants, no literals) — do not refactor it unless required by the grep check
- Adding a row to `specTransitions` or `promptTransitions` in `pkg/spec/spec.go` or `pkg/prompt/prompt.go` must be the ONLY change needed to enable a new transition — no command file should need editing
- The frozen valid-edges tables are unchanged by this prompt — only command-layer usage changes
- Use `errors.Errorf` / `errors.Wrapf` from `github.com/bborbe/errors` for all new error construction in command files
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Verification commands (all must succeed):
```bash
cd /workspace

# Grep checks — both must produce no output (exit 0)
! grep -rn 'Status == "' pkg/cmd/
! grep -rn 'status != "' pkg/cmd/

# Confirm CanTransitionTo is called from command files
grep -rn 'CanTransitionTo' pkg/cmd/

# All tests pass
make test
```

Spot checks:
1. `grep -n 'CanTransitionTo' pkg/cmd/spec_approve.go` — one match
2. `grep -n 'CanTransitionTo' pkg/cmd/spec_unapprove.go` — one match
3. `grep -n 'CanTransitionTo' pkg/cmd/spec_complete.go` — one match
4. `grep -n 'CanTransitionTo' pkg/cmd/unapprove.go` — one match
5. `grep -n 'StatusVerifying' pkg/cmd/spec_list.go` — no literal "verifying" string after the fix
</verification>
