---
status: committing
summary: Added validateListArgs helper in main.go and switched the combined list, prompt list, and spec list dispatchers to use it, so --all passes through to the list commands' own flag parsers; scenario list retains validateNoArgs since it does not support --all; four unit tests added to main_internal_test.go; CHANGELOG updated.
container: dark-factory-335-fix-list-all-flag-dispatch
dark-factory-version: v0.132.0
created: "2026-04-25T14:50:00Z"
queued: "2026-04-25T12:44:50Z"
started: "2026-04-25T12:45:58Z"
---

<summary>
- `dark-factory list --all`, `dark-factory prompt list --all`, `dark-factory spec list --all` all fail with `unexpected argument: "--all"` — the `--all` flag added in spec 058 prompt 332 is rejected by `main.go`'s `validateNoArgs` before the list command can parse it
- Fix: replace `validateNoArgs` with a new `validateListArgs` helper that accepts zero-or-one boolean flag (`--all`), and use it in the three list dispatchers that support `--all`: `list` (combined), `prompt list`, `spec list`. The scenario list dispatcher keeps `validateNoArgs` because `scenario list` does not support `--all`
- Add an end-to-end test that executes each list command with `--all` and asserts no error
- No public API changes; the `--all` flag's documented behaviour is preserved
</summary>

<objective>
Make `--all` actually reach the list commands' flag parser. Today `validateNoArgs` rejects any positional argument starting with `-`, so the parsed `--all` never reaches `listCommand.Run` / `specListCommand.Run` / `combinedListCommand.Run`, even though those Run methods correctly handle it.
</objective>

<context>
**Prerequisite:** Spec 058 (commit v0.135.x) has shipped — `list.go`, `spec_list.go`, `combined_list.go` already parse `--all` correctly inside their `Run` methods. The bug is purely in `main.go` dispatch.

Read `CLAUDE.md` for project conventions.
Read `go-testing-guide.md` from the coding plugin docs (mounted at `/home/node/.claude/plugins/marketplaces/coding/docs/` inside the dark-factory container).

Read these files before editing:
- `main.go` — dispatch table at lines ~96, 125, 206, 263 (and a 5th list at 310 for scenarios). Each calls `validateNoArgs(ctx, args, printXHelp)` BEFORE invoking the list command.
- `main.go` — `validateNoArgs` at line 342: rejects any non-empty `args`.
- `main.go` — `validateOneArg` at line 357: shows the existing pattern for "reject unknown flags" before positional arg parsing — useful reference but NOT the right tool here.
- `pkg/cmd/list.go` — `listCommand.Run`, `--all` parsing at lines 65–80.
- `pkg/cmd/spec_list.go` — `specListCommand.Run`, `--all` parsing at lines 48–55.
- `pkg/cmd/combined_list.go` — combined list runner; check whether it parses `--all` (should after spec 058 prompt 332).

### Bug repro

```bash
$ /tmp/dark-factory spec list --all
error: unexpected argument: "--all"
$ /tmp/dark-factory prompt list --all
error: unexpected argument: "--all"
$ /tmp/dark-factory list --all
error: unexpected argument: "--all"
```

After fix:

```bash
$ /tmp/dark-factory spec list --all     # exits 0, prints rejected entries
$ /tmp/dark-factory prompt list --all   # exits 0, prints rejected entries
$ /tmp/dark-factory list --all          # exits 0, prints rejected entries
```

### Root cause

`validateNoArgs(args)` does:

```go
if len(args) == 0 { return nil }
// otherwise: print help and return error
```

It treats ANY argument as unknown — including `--all`. The list commands' `Run` methods are never reached because the dispatcher exits first.
</context>

<requirements>

## 1. Add a list-specific argument validator in `main.go`

Add a new helper next to `validateNoArgs`:

```go
// validateListArgs returns an error if args contains anything other than the
// optional "--all" flag. Used by the list command dispatchers in main.go.
func validateListArgs(ctx context.Context, args []string, helpFn func()) error {
    for _, arg := range args {
        if arg == "--all" {
            continue
        }
        if strings.HasPrefix(arg, "-") {
            fmt.Fprintf(os.Stderr, "unknown flag: %q\n", arg)
        } else {
            fmt.Fprintf(os.Stderr, "unknown argument: %q\n", arg)
        }
        helpFn()
        return errors.Errorf(ctx, "unexpected argument: %q", arg)
    }
    return nil
}
```

## 2. Use `validateListArgs` instead of `validateNoArgs` for every list dispatch

Find every list dispatcher in `main.go`. Confirmed sites (line numbers approximate — anchor by `case "list":` and surrounding `printXHelp`):

- Combined `list` dispatcher around line 125 — uses `printListHelp`
- `prompt list` dispatcher around line 206 — uses `printPromptHelp`
- `spec list` dispatcher around line 263 — uses `printSpecHelp`
- `scenario list` dispatcher around line 310 — uses `printScenarioHelp`

For EACH of these four sites, replace:

```go
if err := validateNoArgs(ctx, args, printXHelp); err != nil { return err }
```

with:

```go
if err := validateListArgs(ctx, args, printXHelp); err != nil { return err }
```

Verify by grepping after the change:

```bash
grep -n "validateNoArgs.*printListHelp\|validateNoArgs.*printPromptHelp\|validateNoArgs.*printSpecHelp\|validateNoArgs.*printScenarioHelp" main.go
```

That grep should return zero hits — every list dispatcher now uses `validateListArgs`.

If the scenario list command does NOT support `--all` (check `pkg/cmd/scenario_list.go`), use `validateNoArgs` for it and only switch the three list dispatchers that do.

## 3. Tests

### 3a. Argument-validator unit tests in `main_internal_test.go`

(`validateListArgs` is unexported, so tests live in the internal package — same file as existing `validateNoArgs` tests.)

Add tests for `validateListArgs`:

```go
It("validateListArgs accepts --all", func() {
    Expect(validateListArgs(ctx, []string{"--all"}, noopHelp)).To(Succeed())
})
It("validateListArgs accepts no args", func() {
    Expect(validateListArgs(ctx, []string{}, noopHelp)).To(Succeed())
})
It("validateListArgs rejects unknown flags", func() {
    Expect(validateListArgs(ctx, []string{"--foo"}, noopHelp)).To(HaveOccurred())
})
It("validateListArgs rejects positional arguments", func() {
    Expect(validateListArgs(ctx, []string{"spec-name"}, noopHelp)).To(HaveOccurred())
})
```

Use the same test pattern that exists for `validateNoArgs`. If existing tests use a different framework (e.g. plain `testing.T` table tests), match that.

### 3b. End-to-end CLI smoke (regression test)

This bug slipped past unit tests because the unit tests verified `Run` accepts `--all` but never tested the dispatch path. Add a smoke test in the existing CLI integration test file (or create a new `main_e2e_test.go` if none exists).

The test should invoke the binary's main entrypoint (or, if testing via `go test ./...`, call the dispatch function directly with `args = []string{"spec", "list", "--all"}`) and assert no error is returned.

If the project does not currently have e2e tests at the dispatch layer, add at minimum a unit test that calls the dispatch function directly with `--all` for each of the three (or four) list paths.

## 4. CHANGELOG entry

Append to `## Unreleased` in `CHANGELOG.md`:

```
- fix: dark-factory `list --all`, `prompt list --all`, `spec list --all`, and `scenario list --all` (if applicable) no longer reject the flag at dispatch time — the new `validateListArgs` helper accepts `--all` and lets the list command's own parser handle it
```

## 5. Run verification

```bash
cd /workspace && make precommit
```

Must exit 0.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- The new `validateListArgs` helper is the ONLY new function; do not change `validateNoArgs` (other commands still use it correctly)
- All existing tests must pass unchanged
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for any new error construction (or `errors.Errorf` for new errors without an underlying cause)
- External test packages where applicable
- Coverage ≥80% for the new `validateListArgs` function (do not chase coverage in unrelated parts of `main.go`)
- Whether `scenario list` supports `--all` is a runtime check — read `pkg/cmd/scenario_list.go` first; if it does not parse `--all`, leave the scenario dispatcher on `validateNoArgs`
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:

```bash
cd /workspace

# New helper exists
grep -n "func validateListArgs" main.go

# All list dispatchers use the new helper (or scenario list is documented exception)
grep -n "validateListArgs" main.go

# Old validator no longer used for list dispatch (scenario list intentionally still uses validateNoArgs)
! grep -n "validateNoArgs.*printListHelp\|validateNoArgs.*printPromptHelp\|validateNoArgs.*printSpecHelp" main.go
# scenario list keeps validateNoArgs — confirm
grep -n "validateNoArgs.*printScenarioHelp" main.go

# E2E test present
grep -n "spec.*list.*--all\|list --all" main_e2e_test.go main_internal_test.go 2>/dev/null
```
</verification>
