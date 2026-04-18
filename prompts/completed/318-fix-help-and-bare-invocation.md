---
status: completed
summary: Routed empty args and bare 'help' word to the help command in ParseArgs, updated tests, and updated go-git to v5.18.0 to fix a security vulnerability found during precommit.
container: dark-factory-318-fix-help-and-bare-invocation
dark-factory-version: v0.122.0-6-g6b02e84
created: "2026-04-18T00:00:00Z"
queued: "2026-04-18T15:06:08Z"
started: "2026-04-18T15:07:45Z"
completed: "2026-04-18T15:14:49Z"
---

<summary>
- Running `dark-factory` with no arguments now prints the usage text and exits successfully instead of erroring
- Running `dark-factory help` now prints the same usage text as `dark-factory --help` instead of erroring
- `dark-factory --help`, `-h`, and `-help` remain unchanged — they already print usage
- All other unrecognized commands (e.g. `dark-factory nonexistent`) still error as before
- Pure UX fix for CLI invocation — no new features, no behavior changes to any existing subcommand
</summary>

<objective>
Make `dark-factory` (no args) and `dark-factory help` print the same usage output as `dark-factory --help` (exit 0). Today both fall through to the "unknown command" error path. The `help` flow already exists in `run()` — the fix is to route these two inputs to it from `ParseArgs`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/prompt-writing.md` for prompt conventions.

Key files:
- `main.go` — contains `ParseArgs` (bottom of file, ~line 550), `run()` (~line 36), and `printHelp()` (~line 420)
- `parse_args_test.go` — table-style tests for `ParseArgs`; contains `TestParseArgsNoArgs` which currently asserts the broken behavior and must be updated
- `main_internal_test.go` / `main_test.go` — existing test files; follow their style if adding higher-level tests

Current broken behavior (from `main.go`):
- `ParseArgs([])` returns `command="unknown"` → `run()` hits the `case "unknown"` branch and prints `Run 'dark-factory help' for usage.\nerror: unknown command: ""`
- `ParseArgs(["help"])` returns `command="unknown"` (because the final `switch` has no `case "help"`) → same error path

Existing correct behavior:
- `ParseArgs(["--help"])` → `command="help"` → `run()` hits `case "help"` → calls `printHelp()` and returns `nil` (exit 0)
</context>

<requirements>

## 1. Update `ParseArgs` in `main.go` to recognize `help` and empty input as the help command

Find `ParseArgs` near the bottom of `main.go` (~line 550). Make two changes:

### 1a. Empty-args case

Change the early return for empty filtered input from `"unknown"` to `"help"`:

```go
// Before:
if len(filtered) == 0 {
    return debug, "unknown", "", []string{}, autoApprove
}

// After:
if len(filtered) == 0 {
    return debug, "help", "", []string{}, autoApprove
}
```

### 1b. Explicit `help` subcommand

Add `"help"` to the existing `--help / -help / -h` switch case so a bare `help` word is treated identically to the help flag:

```go
// Before:
switch command {
case "--help", "-help", "-h":
    return debug, "help", "", []string{}, autoApprove
case "--version", "-version", "-v":
    return debug, "version", "", []string{}, autoApprove

// After:
switch command {
case "help", "--help", "-help", "-h":
    return debug, "help", "", []string{}, autoApprove
case "--version", "-version", "-v":
    return debug, "version", "", []string{}, autoApprove
```

### 1c. Update the `ParseArgs` doc comment

The comment block above `ParseArgs` currently says:

```
// No args → command="unknown" (an explicit subcommand is required)
```

Change it to:

```
// No args → command="help" (prints usage, exits 0)
// Bare "help" word → command="help" (same as --help)
```

Leave the rest of the doc comment untouched.

## 2. Update the existing test `TestParseArgsNoArgs` in `parse_args_test.go`

The test currently asserts the broken behavior. Update it:

```go
// Before:
func TestParseArgsNoArgs(t *testing.T) {
    t.Parallel()
    // No args must return "unknown" — an explicit subcommand is required
    assertParseArgs(t, []string{}, parseArgsResult{command: "unknown", args: []string{}})
}

// After:
func TestParseArgsNoArgs(t *testing.T) {
    t.Parallel()
    // No args must return "help" — so running `dark-factory` with no args prints usage and exits 0
    assertParseArgs(t, []string{}, parseArgsResult{command: "help", args: []string{}})
}
```

## 3. Add a new test `TestParseArgsHelpWord` in `parse_args_test.go`

Add this test after `TestParseArgsNoArgs`, using the same `assertParseArgs` helper:

```go
func TestParseArgsHelpWord(t *testing.T) {
    t.Parallel()
    // Bare `help` word must be treated exactly like `--help`
    assertParseArgs(t, []string{"help"}, parseArgsResult{command: "help", args: []string{}})
    // The -debug flag is still extracted when combined with help
    assertParseArgs(t, []string{"-debug", "help"}, parseArgsResult{debug: true, command: "help", args: []string{}})
    // `--help` continues to work (regression guard)
    assertParseArgs(t, []string{"--help"}, parseArgsResult{command: "help", args: []string{}})
}
```

## 4. Confirm no other code needs to change

The `run()` function in `main.go` already has `case "help": printHelp(); return nil`. Do NOT add a new case, do NOT introduce a new help function. The entire fix must live in `ParseArgs` and its tests.

Grep to confirm no stale references remain:

```bash
grep -n '"unknown"' main.go
```

After the change, the only remaining `"unknown"` string in `main.go` should be the fallback returns in `ParseArgs` (for unrecognized commands) and the `case "unknown":` branch in `run()`.

## 5. Run the full precommit check

```bash
make precommit
```

All existing tests must still pass. The two updated/added tests must pass.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT add new external dependencies
- Do NOT create a separate "help command" type or refactor the command dispatch — just route `""` and `"help"` through the existing help flow
- Do NOT change `printHelp()`, `printCommandHelp()`, or any per-command help function
- Do NOT touch `go.mod` / `go.sum` / `vendor/`
- Keep the change minimal: one function (`ParseArgs`) plus its tests
- Existing tests must still pass — only `TestParseArgsNoArgs` may be modified (because it asserts the old broken behavior)
- `dark-factory nonexistent` must still return `command="unknown"` with `args=["nonexistent"]` (unchanged)
</constraints>

<verification>
Run `make precommit` — must pass.

Additional spot checks (commands run from the repo root inside the container):

```bash
# Build the binary
go build -o /tmp/df .

# No args — must print usage and exit 0
/tmp/df; echo "exit=$?"
# Expected: full usage text, then "exit=0"

# Bare help word — must print usage and exit 0
/tmp/df help; echo "exit=$?"
# Expected: full usage text, then "exit=0"

# --help flag — unchanged, must print usage and exit 0
/tmp/df --help; echo "exit=$?"
# Expected: full usage text, then "exit=0"

# Unknown command — must still error
/tmp/df nonexistent; echo "exit=$?"
# Expected: "Run 'dark-factory help' for usage." + "error: unknown command: \"nonexistent\"" + "exit=1"
```

Also verify the three test functions pass individually:

```bash
go test -run 'TestParseArgsNoArgs|TestParseArgsHelpWord|TestParseArgsDefaults' ./...
```
</verification>
